# TODO - Cloak Apps

## Phase 2: Server-Side User Preferences Persistence

### Overview
Currently using localStorage for client-side persistence of user preferences (theme, view mode). Once Keycloak authentication is implemented, migrate to server-side persistence while maintaining localStorage as fallback for anonymous users.

### Prerequisites
- [ ] Keycloak SSO integration completed
- [ ] User authentication flow working
- [ ] User ID available in session/context

---

### Implementation Steps

#### 1. Database Schema

Create `user_preferences` table:

```sql
CREATE TABLE user_preferences (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(255) UNIQUE NOT NULL,  -- Keycloak user ID
    theme VARCHAR(10) DEFAULT 'dark',      -- 'dark' or 'light'
    view_mode VARCHAR(10) DEFAULT 'card',  -- 'card' or 'list'
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_user_preferences_user_id ON user_preferences(user_id);
```

**Notes:**
- Use Keycloak's `sub` claim as `user_id`
- Consider adding `UNIQUE` constraint on `user_id`
- Add `updated_at` trigger for automatic timestamp updates

---

#### 2. Repository Layer (`internal/repository/preferences.go`)

```go
package repository

import (
    "context"
    "database/sql"
)

type UserPreferences struct {
    UserID   string
    Theme    string // "dark" or "light"
    ViewMode string // "card" or "list"
}

type PreferencesRepository interface {
    GetByUserID(ctx context.Context, userID string) (*UserPreferences, error)
    Upsert(ctx context.Context, prefs *UserPreferences) error
}

type preferencesRepo struct {
    db *sql.DB
}

func NewPreferencesRepository(db *sql.DB) PreferencesRepository {
    return &preferencesRepo{db: db}
}

func (r *preferencesRepo) GetByUserID(ctx context.Context, userID string) (*UserPreferences, error) {
    // Implementation: SELECT from user_preferences WHERE user_id = $1
    // Return default values if not found (dark theme, card view)
}

func (r *preferencesRepo) Upsert(ctx context.Context, prefs *UserPreferences) error {
    // Implementation: INSERT ... ON CONFLICT (user_id) DO UPDATE
}
```

**Notes:**
- Use prepared statements to prevent SQL injection
- Handle `sql.ErrNoRows` gracefully by returning default preferences
- Consider adding caching layer (Redis) for high-traffic scenarios

---

#### 3. Service Layer (`internal/service/preferences.go`)

```go
package service

import (
    "context"
    "github.com/effiware/cloak-apps/internal/repository"
)

type PreferencesService struct {
    repo repository.PreferencesRepository
}

func NewPreferencesService(repo repository.PreferencesRepository) *PreferencesService {
    return &PreferencesService{repo: repo}
}

func (s *PreferencesService) GetUserPreferences(ctx context.Context, userID string) (*repository.UserPreferences, error) {
    prefs, err := s.repo.GetByUserID(ctx, userID)
    if err != nil {
        // Return defaults on error
        return &repository.UserPreferences{
            UserID:   userID,
            Theme:    "dark",
            ViewMode: "card",
        }, nil
    }
    return prefs, nil
}

func (s *PreferencesService) UpdateTheme(ctx context.Context, userID, theme string) error {
    // Validate theme value
    if theme != "dark" && theme != "light" {
        return errors.New("invalid theme value")
    }

    prefs, _ := s.GetUserPreferences(ctx, userID)
    prefs.Theme = theme
    return s.repo.Upsert(ctx, prefs)
}

func (s *PreferencesService) UpdateViewMode(ctx context.Context, userID, viewMode string) error {
    // Validate viewMode value
    if viewMode != "card" && viewMode != "list" {
        return errors.New("invalid view mode value")
    }

    prefs, _ := s.GetUserPreferences(ctx, userID)
    prefs.ViewMode = viewMode
    return s.repo.Upsert(ctx, prefs)
}
```

**Notes:**
- Add validation for preference values
- Consider adding business logic (e.g., rate limiting preference updates)
- Log errors but don't fail hard - UX should gracefully degrade

---

#### 4. Handler Layer (`internal/handlers/preferences.go`)

```go
package handlers

import (
    "net/http"
    "github.com/effiware/cloak-apps/internal/service"
)

type PreferencesHandler struct {
    service *service.PreferencesService
}

func NewPreferencesHandler(service *service.PreferencesService) *PreferencesHandler {
    return &PreferencesHandler{service: service}
}

// UpdateTheme handles HTMX request to update theme preference
// POST /preferences/theme
func (h *PreferencesHandler) UpdateTheme(w http.ResponseWriter, r *http.Request) {
    userID := getUserIDFromContext(r.Context()) // Extract from Keycloak token
    theme := r.FormValue("theme")

    if err := h.service.UpdateTheme(r.Context(), userID, theme); err != nil {
        http.Error(w, "Failed to update theme", http.StatusInternalServerError)
        return
    }

    // Return empty 200 response for HTMX
    // Client-side Alpine.js handles the visual update
    w.Header().Set("HX-Trigger", "theme-updated")
    w.WriteHeader(http.StatusOK)
}

// UpdateViewMode handles HTMX request to update view mode preference
// POST /preferences/view-mode
func (h *PreferencesHandler) UpdateViewMode(w http.ResponseWriter, r *http.Request) {
    userID := getUserIDFromContext(r.Context())
    viewMode := r.FormValue("view_mode")

    if err := h.service.UpdateViewMode(r.Context(), userID, viewMode); err != nil {
        http.Error(w, "Failed to update view mode", http.StatusInternalServerError)
        return
    }

    w.Header().Set("HX-Trigger", "view-mode-updated")
    w.WriteHeader(http.StatusOK)
}
```

**Notes:**
- Use middleware to extract and validate Keycloak JWT
- Add CSRF protection
- Consider adding optimistic UI updates on client before server confirmation

---

#### 5. Templ Integration (`internal/views/index.templ`)

```go
templ Index(prefs *repository.UserPreferences) {
    <html lang="en" class={prefs.Theme}>
        <head>
            <!-- ... -->
        </head>
        <body
            class="bg-gray-50 dark:bg-gray-900 min-h-screen"
            x-data={initializeAppState(prefs)}
            x-init="syncThemeClass()">

            <!-- Dark Mode Toggle with HTMX -->
            <button
                @click="toggleTheme()"
                hx-post="/preferences/theme"
                hx-vals='{"theme": darkMode ? "light" : "dark"}'
                hx-trigger="click"
                hx-swap="none"
                class="p-2 rounded-lg bg-gray-100 dark:bg-gray-700"
                title="Toggle dark mode">
                <!-- Icons -->
            </button>

            <!-- View Mode Toggle with HTMX -->
            <button
                @click="viewMode = 'card'"
                hx-post="/preferences/view-mode"
                hx-vals='{"view_mode": "card"}'
                hx-trigger="click"
                hx-swap="none">
                <!-- Card icon -->
            </button>
        </body>
    </html>
}

script initializeAppState(prefs *repository.UserPreferences) string {
    return fmt.Sprintf(`{
        darkMode: %t,
        viewMode: '%s',
        userMenuOpen: false,

        // Sync localStorage with server state
        init() {
            localStorage.setItem('theme', this.darkMode ? 'dark' : 'light');
            localStorage.setItem('viewMode', this.viewMode);
        },

        toggleTheme() {
            this.darkMode = !this.darkMode;
            this.syncThemeClass();
            localStorage.setItem('theme', this.darkMode ? 'dark' : 'light');
        },

        syncThemeClass() {
            if (this.darkMode) {
                document.documentElement.classList.add('dark');
            } else {
                document.documentElement.classList.remove('dark');
            }
        }
    }`,
    prefs.Theme == "dark",
    prefs.ViewMode)
}
```

**Notes:**
- Server-rendered initial state from database
- Alpine.js syncs localStorage with server state
- HTMX sends preference updates asynchronously
- Client-side update is immediate (optimistic UI)
- Server persists in background

---

#### 6. Router Configuration (`cmd/server/main.go` or router setup)

```go
func setupRoutes(r *chi.Mux, handlers *Handlers) {
    // ... existing routes

    // Preferences routes (authenticated only)
    r.Group(func(r chi.Router) {
        r.Use(authMiddleware) // Keycloak JWT validation
        r.Post("/preferences/theme", handlers.Preferences.UpdateTheme)
        r.Post("/preferences/view-mode", handlers.Preferences.UpdateViewMode)
    })
}
```

**Notes:**
- Protect preference endpoints with authentication middleware
- Consider rate limiting to prevent abuse
- Add request logging for debugging

---

### Migration Strategy

1. **Backward Compatibility:**
   - Keep localStorage as fallback for unauthenticated users
   - Check if user is authenticated before making server calls
   - Gracefully degrade if server is unavailable

2. **User Migration:**
   - On first login after Phase 2 deployment:
     - Read from localStorage
     - If preferences exist, sync to server
     - Clear migration flag

3. **Feature Flag:**
   - Use feature flag to toggle between localStorage-only and server persistence
   - Allows gradual rollout and easy rollback

```javascript
x-init="
    if (isAuthenticated && featureFlags.serverPreferences) {
        // Use server-rendered preferences
        syncThemeClass();
    } else {
        // Fall back to localStorage
        const savedTheme = localStorage.getItem('theme');
        this.darkMode = savedTheme === 'dark' || savedTheme === null;
        syncThemeClass();
    }
"
```

---

### Testing Checklist

- [ ] Unit tests for repository layer (mocked DB)
- [ ] Unit tests for service layer (mocked repo)
- [ ] Integration tests for handlers (test server)
- [ ] E2E tests for preference persistence flow
- [ ] Test unauthenticated user fallback
- [ ] Test network failure scenarios
- [ ] Test concurrent preference updates
- [ ] Load testing for high-traffic scenarios

---

### Performance Considerations

1. **Caching:**
   - Consider Redis cache for user preferences
   - Cache invalidation on update
   - TTL of 1 hour

2. **Database Indexing:**
   - Index on `user_id` for fast lookups
   - Consider partitioning if user base grows large

3. **Connection Pooling:**
   - Configure appropriate DB connection pool size
   - Monitor slow queries

---

### Security Considerations

- [ ] Validate Keycloak JWT on all preference endpoints
- [ ] Add CSRF protection
- [ ] Rate limit preference update endpoints (e.g., 10 updates/minute per user)
- [ ] Sanitize all user input (theme/viewMode values)
- [ ] Log preference changes for audit trail
- [ ] Use parameterized queries to prevent SQL injection

---

### Monitoring & Observability

- [ ] Add metrics for preference update latency
- [ ] Monitor database connection pool usage
- [ ] Track preference update success/failure rates
- [ ] Alert on high error rates
- [ ] Dashboard for most popular preferences (dark vs light)

---

### Documentation

- [ ] Update API documentation with preference endpoints
- [ ] Document preference data model
- [ ] Add architecture decision record (ADR) for preference storage approach
- [ ] Update user guide with preference customization features

---

## Additional Future Enhancements

### Phase 3 (Optional): Advanced Preferences

Consider adding more user preferences:
- [ ] Language/locale preference
- [ ] Timezone preference
- [ ] Items per page preference
- [ ] Default sort order
- [ ] Notification preferences
- [ ] Accessibility preferences (reduced motion, high contrast, etc.)

### Phase 4 (Optional): Preference Sync Across Devices

- [ ] Real-time preference sync using WebSockets
- [ ] Conflict resolution for concurrent updates
- [ ] Last-write-wins strategy with timestamp

---

## References

- [HDA Architecture](https://htmx.org/essays/hypermedia-driven-applications/)
- [Templ Documentation](https://templ.guide/)
- [Alpine.js Persistence](https://alpinejs.dev/plugins/persist)
- [Keycloak JWT Token Structure](https://www.keycloak.org/docs/latest/server_admin/#_token-exchange)
- [Chi Router Middleware](https://github.com/go-chi/chi)

---

## Phase 2: Application Attributes & Filtering System

### Overview

Implement a multi-dimensional classification system for applications using structured attributes. Applications are grouped by a primary dimension (e.g., "space") and can be filtered by secondary dimensions (e.g., "environment"). The system supports wildcard values (like "all") and is extensible for future attribute types.

**Current State:** Mock data with hardcoded attributes and client-side filtering using Alpine.js

**Phase 2 Goal:** Move attribute definitions and application data to server-side with dynamic filtering

### Prerequisites

- [ ] Database schema for applications and attributes
- [ ] Application repository layer
- [ ] Basic application CRUD operations
- [ ] Keycloak integration (for ABAC - optional but recommended)

---

### Terminology & Concepts

**"Attributes"** (chosen over "tags", "properties", "metadata"):
- Structured key-value pairs
- Validated against a schema
- Familiar to engineers (IAM, RBAC, ABAC systems)
- Natural fit with Keycloak token attributes
- Clear distinction from free-form tags

**Attribute Roles:**
1. **Grouping Attributes** - Primary dimension, always visible (e.g., "space")
2. **Filtering Attributes** - Secondary dimensions, user-selectable (e.g., "environment")

**Wildcard Support:**
- Some attributes support wildcard values (e.g., "all")
- Apps with "all" match any filter value
- Filter "all" shows all apps regardless of their value

---

### Data Model

#### 1. Application Structure

```go
package model

type Application struct {
    ID          string            `json:"id" db:"id"`
    Name        string            `json:"name" db:"name"`
    Description string            `json:"description" db:"description"`
    Icon        string            `json:"icon" db:"icon"`           // Emoji or icon URL
    URL         string            `json:"url" db:"url"`
    Attributes  map[string]string `json:"attributes" db:"attributes"` // JSONB in PostgreSQL
    Enabled     bool              `json:"enabled" db:"enabled"`
    CreatedAt   time.Time         `json:"created_at" db:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at" db:"updated_at"`
}

// Example
{
    "id": "prometheus",
    "name": "Prometheus",
    "description": "Monitoring system and time series database",
    "icon": "🔥",
    "url": "https://prometheus.company.com",
    "attributes": {
        "space": "operations",
        "environment": "all"
    },
    "enabled": true
}
```

#### 2. Attribute Schema Definition

```go
package model

type AttributeDefinition struct {
    Key              string        `json:"key" db:"key"`
    DisplayLabel     string        `json:"display_label" db:"display_label"`
    Role             AttributeRole `json:"role" db:"role"`
    Values           []string      `json:"values" db:"values"`           // JSONB array
    SupportsWildcard bool          `json:"supports_wildcard" db:"supports_wildcard"`
    DefaultValue     string        `json:"default_value" db:"default_value"`
    SortOrder        int           `json:"sort_order" db:"sort_order"`    // For UI ordering
    CreatedAt        time.Time     `json:"created_at" db:"created_at"`
    UpdatedAt        time.Time     `json:"updated_at" db:"updated_at"`
}

type AttributeRole string

const (
    RoleGrouping  AttributeRole = "grouping"
    RoleFiltering AttributeRole = "filtering"
)

// Example schema
var DefaultAttributeSchema = []AttributeDefinition{
    {
        Key:              "space",
        DisplayLabel:     "Space",
        Role:             RoleGrouping,
        Values:           []string{"operations", "tools", "mvp", "alpha"},
        SupportsWildcard: false,
        SortOrder:        1,
    },
    {
        Key:              "environment",
        DisplayLabel:     "Environment",
        Role:             RoleFiltering,
        Values:           []string{"production", "development", "all"},
        SupportsWildcard: true,
        DefaultValue:     "all",
        SortOrder:        2,
    },
}
```

---

### Database Schema

```sql
-- Applications table
CREATE TABLE applications (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    icon VARCHAR(10),                    -- Emoji or short icon identifier
    url TEXT NOT NULL,
    attributes JSONB NOT NULL DEFAULT '{}',
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Index for attribute queries
CREATE INDEX idx_applications_attributes ON applications USING GIN (attributes);

-- Index for enabled apps
CREATE INDEX idx_applications_enabled ON applications(enabled) WHERE enabled = TRUE;

-- Attribute definitions table
CREATE TABLE attribute_definitions (
    key VARCHAR(255) PRIMARY KEY,
    display_label VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL,           -- 'grouping' or 'filtering'
    values JSONB NOT NULL,               -- Array of valid values
    supports_wildcard BOOLEAN DEFAULT FALSE,
    default_value VARCHAR(255),
    sort_order INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Seed data for attribute definitions
INSERT INTO attribute_definitions (key, display_label, role, values, supports_wildcard, default_value, sort_order)
VALUES
    ('space', 'Space', 'grouping', '["operations", "tools", "mvp", "alpha"]', false, NULL, 1),
    ('environment', 'Environment', 'filtering', '["production", "development", "all"]', true, 'all', 2);

-- Seed example applications
INSERT INTO applications (id, name, description, icon, url, attributes)
VALUES
    ('prometheus', 'Prometheus', 'Monitoring system and time series database', '🔥', 'https://prometheus.company.com', '{"space": "operations", "environment": "all"}'),
    ('grafana', 'Grafana', 'Analytics and interactive visualization platform', '📊', 'https://grafana.company.com', '{"space": "operations", "environment": "all"}'),
    ('keycloak', 'Keycloak', 'Identity and access management solution', '🔐', 'https://keycloak.company.com', '{"space": "tools", "environment": "all"}'),
    ('mattermost', 'Mattermost', 'Secure collaboration platform for teams', '💬', 'https://mattermost.company.com', '{"space": "tools", "environment": "production"}'),
    ('vaultwarden', 'Vaultwarden', 'Password management and secure storage', '🔒', 'https://vault.company.com', '{"space": "tools", "environment": "production"}'),
    ('docmost', 'Docmost', 'Collaborative documentation platform', '📝', 'https://docs.company.com', '{"space": "mvp", "environment": "development"}');
```

**Notes:**
- Use JSONB for attributes to allow flexible querying and indexing
- GIN index enables efficient attribute-based queries
- Consider adding audit columns (created_by, updated_by) if needed
- Add triggers for `updated_at` timestamps

---

### Repository Layer

#### `internal/repository/application.go`

```go
package repository

import (
    "context"
    "database/sql"
    "encoding/json"

    "github.com/effiware/cloak-apps/internal/model"
)

type ApplicationRepository interface {
    GetAll(ctx context.Context) ([]*model.Application, error)
    GetByID(ctx context.Context, id string) (*model.Application, error)
    GetByAttributes(ctx context.Context, filters map[string]string) ([]*model.Application, error)
    Create(ctx context.Context, app *model.Application) error
    Update(ctx context.Context, app *model.Application) error
    Delete(ctx context.Context, id string) error
}

type applicationRepo struct {
    db *sql.DB
}

func NewApplicationRepository(db *sql.DB) ApplicationRepository {
    return &applicationRepo{db: db}
}

func (r *applicationRepo) GetAll(ctx context.Context) ([]*model.Application, error) {
    query := `SELECT id, name, description, icon, url, attributes, enabled, created_at, updated_at
              FROM applications WHERE enabled = TRUE ORDER BY name`

    rows, err := r.db.QueryContext(ctx, query)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var apps []*model.Application
    for rows.Next() {
        app := &model.Application{}
        var attrsJSON []byte

        err := rows.Scan(&app.ID, &app.Name, &app.Description, &app.Icon,
                        &app.URL, &attrsJSON, &app.Enabled, &app.CreatedAt, &app.UpdatedAt)
        if err != nil {
            return nil, err
        }

        if err := json.Unmarshal(attrsJSON, &app.Attributes); err != nil {
            return nil, err
        }

        apps = append(apps, app)
    }

    return apps, rows.Err()
}

func (r *applicationRepo) GetByAttributes(ctx context.Context, filters map[string]string) ([]*model.Application, error) {
    // Build dynamic query based on filters
    // Handle wildcard matching (app with "all" matches any filter)

    query := `SELECT id, name, description, icon, url, attributes, enabled, created_at, updated_at
              FROM applications
              WHERE enabled = TRUE`

    args := []interface{}{}
    argPos := 1

    for key, value := range filters {
        // Skip "all" filters - they match everything
        if value == "all" {
            continue
        }

        // Match either exact value OR "all" wildcard
        query += fmt.Sprintf(` AND (attributes->>'%s' = $%d OR attributes->>'%s' = 'all')`,
                             key, argPos, key)
        args = append(args, value)
        argPos++
    }

    query += ` ORDER BY name`

    // Execute query and scan results (similar to GetAll)
    // ...
}
```

#### `internal/repository/attribute.go`

```go
package repository

import (
    "context"
    "database/sql"

    "github.com/effiware/cloak-apps/internal/model"
)

type AttributeRepository interface {
    GetAll(ctx context.Context) ([]*model.AttributeDefinition, error)
    GetByKey(ctx context.Context, key string) (*model.AttributeDefinition, error)
    GetByRole(ctx context.Context, role model.AttributeRole) ([]*model.AttributeDefinition, error)
    Create(ctx context.Context, attr *model.AttributeDefinition) error
    Update(ctx context.Context, attr *model.AttributeDefinition) error
}

type attributeRepo struct {
    db *sql.DB
}

func NewAttributeRepository(db *sql.DB) AttributeRepository {
    return &attributeRepo{db: db}
}

func (r *attributeRepo) GetAll(ctx context.Context) ([]*model.AttributeDefinition, error) {
    query := `SELECT key, display_label, role, values, supports_wildcard, default_value, sort_order, created_at, updated_at
              FROM attribute_definitions ORDER BY sort_order, key`

    // Query and scan results
    // ...
}

func (r *attributeRepo) GetByRole(ctx context.Context, role model.AttributeRole) ([]*model.AttributeDefinition, error) {
    query := `SELECT key, display_label, role, values, supports_wildcard, default_value, sort_order, created_at, updated_at
              FROM attribute_definitions WHERE role = $1 ORDER BY sort_order, key`

    // Query and scan results
    // ...
}
```

---

### Service Layer

#### `internal/service/application.go`

```go
package service

import (
    "context"

    "github.com/effiware/cloak-apps/internal/model"
    "github.com/effiware/cloak-apps/internal/repository"
)

type ApplicationService struct {
    appRepo  repository.ApplicationRepository
    attrRepo repository.AttributeRepository
}

func NewApplicationService(appRepo repository.ApplicationRepository, attrRepo repository.AttributeRepository) *ApplicationService {
    return &ApplicationService{
        appRepo:  appRepo,
        attrRepo: attrRepo,
    }
}

// GetApplicationsGrouped returns apps grouped by the grouping attribute
func (s *ApplicationService) GetApplicationsGrouped(ctx context.Context, filters map[string]string) (map[string][]*model.Application, error) {
    // Get all apps matching filters
    apps, err := s.appRepo.GetByAttributes(ctx, filters)
    if err != nil {
        return nil, err
    }

    // Get grouping attribute definition
    groupingAttrs, err := s.attrRepo.GetByRole(ctx, model.RoleGrouping)
    if err != nil || len(groupingAttrs) == 0 {
        return nil, err
    }

    groupingKey := groupingAttrs[0].Key

    // Group apps by grouping attribute value
    grouped := make(map[string][]*model.Application)
    for _, app := range apps {
        groupValue := app.Attributes[groupingKey]
        if groupValue == "" {
            groupValue = "other" // Fallback for apps without grouping attribute
        }
        grouped[groupValue] = append(grouped[groupValue], app)
    }

    return grouped, nil
}

// GetFilteringAttributes returns all filtering attribute definitions
func (s *ApplicationService) GetFilteringAttributes(ctx context.Context) ([]*model.AttributeDefinition, error) {
    return s.attrRepo.GetByRole(ctx, model.RoleFiltering)
}

// ValidateAttributes checks if app attributes are valid
func (s *ApplicationService) ValidateAttributes(ctx context.Context, attributes map[string]string) error {
    attrDefs, err := s.attrRepo.GetAll(ctx)
    if err != nil {
        return err
    }

    attrDefMap := make(map[string]*model.AttributeDefinition)
    for _, def := range attrDefs {
        attrDefMap[def.Key] = def
    }

    for key, value := range attributes {
        def, exists := attrDefMap[key]
        if !exists {
            return fmt.Errorf("unknown attribute key: %s", key)
        }

        // Check if value is valid
        validValue := false
        for _, allowedValue := range def.Values {
            if value == allowedValue {
                validValue = true
                break
            }
        }

        if !validValue {
            return fmt.Errorf("invalid value '%s' for attribute '%s'", value, key)
        }
    }

    return nil
}
```

---

### Handler Layer

#### `internal/handlers/application.go`

```go
package handlers

import (
    "net/http"

    "github.com/effiware/cloak-apps/internal/service"
    "github.com/effiware/cloak-apps/internal/views"
)

type ApplicationHandler struct {
    service *service.ApplicationService
}

func NewApplicationHandler(service *service.ApplicationService) *ApplicationHandler {
    return &ApplicationHandler{service: service}
}

// Index renders the main applications page
// GET /
func (h *ApplicationHandler) Index(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Get filtering attributes
    filteringAttrs, err := h.service.GetFilteringAttributes(ctx)
    if err != nil {
        http.Error(w, "Failed to load filters", http.StatusInternalServerError)
        return
    }

    // Parse filter query params
    filters := make(map[string]string)
    for _, attr := range filteringAttrs {
        if val := r.URL.Query().Get(attr.Key); val != "" {
            filters[attr.Key] = val
        } else if attr.DefaultValue != "" {
            filters[attr.Key] = attr.DefaultValue
        }
    }

    // Get grouped applications
    groupedApps, err := h.service.GetApplicationsGrouped(ctx, filters)
    if err != nil {
        http.Error(w, "Failed to load applications", http.StatusInternalServerError)
        return
    }

    // Render templ view
    component := views.Index(groupedApps, filteringAttrs, filters)
    component.Render(ctx, w)
}

// FilterApplications handles HTMX filter requests
// GET /applications/filter?environment=production
func (h *ApplicationHandler) FilterApplications(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Parse filters from query params
    filters := make(map[string]string)
    for key, values := range r.URL.Query() {
        if len(values) > 0 {
            filters[key] = values[0]
        }
    }

    // Get grouped applications
    groupedApps, err := h.service.GetApplicationsGrouped(ctx, filters)
    if err != nil {
        http.Error(w, "Failed to filter applications", http.StatusInternalServerError)
        return
    }

    // Render just the applications section (for HTMX swap)
    component := views.ApplicationsGrid(groupedApps)
    component.Render(ctx, w)
}
```

---

### Templ Views

#### `internal/views/index.templ`

```go
package views

import "github.com/effiware/cloak-apps/internal/model"

templ Index(groupedApps map[string][]*model.Application, filteringAttrs []*model.AttributeDefinition, activeFilters map[string]string) {
    <html lang="en" class="dark">
        <head>
            <!-- ... head content ... -->
        </head>
        <body class="bg-gray-50 dark:bg-gray-900 min-h-screen" x-data="{ /* ... */ }">
            <!-- Navbar -->
            @Navbar()

            <!-- Main Content -->
            <main class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
                <!-- Header with Filters -->
                <div class="mb-8">
                    <h1 class="text-3xl font-bold text-gray-900 dark:text-white mb-4">My Applications</h1>

                    <!-- Filtering Pills -->
                    @FilteringPills(filteringAttrs, activeFilters)
                </div>

                <!-- Applications Grid -->
                <div id="applications-container">
                    @ApplicationsGrid(groupedApps)
                </div>
            </main>
        </body>
    </html>
}

templ FilteringPills(filteringAttrs []*model.AttributeDefinition, activeFilters map[string]string) {
    for _, attr := range filteringAttrs {
        <div class="mb-4">
            <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                {attr.DisplayLabel}
            </label>
            <div class="flex flex-wrap gap-2">
                for _, value := range attr.Values {
                    @FilterPill(attr.Key, value, value == activeFilters[attr.Key])
                }
            </div>
        </div>
    }
}

templ FilterPill(attrKey string, value string, active bool) {
    <button
        hx-get={"/applications/filter?" + attrKey + "=" + value}
        hx-target="#applications-container"
        hx-swap="innerHTML"
        class={
            "px-4 py-2 rounded-full text-sm font-medium transition-colors " +
            if active {
                "bg-blue-600 text-white"
            } else {
                "bg-gray-200 dark:bg-gray-700 text-gray-800 dark:text-gray-200 hover:bg-gray-300 dark:hover:bg-gray-600"
            }
        }>
        {value}
    </button>
}

templ ApplicationsGrid(groupedApps map[string][]*model.Application) {
    for groupName, apps := range groupedApps {
        if len(apps) > 0 {
            <div class="mb-8">
                <h2 class="text-xl font-semibold text-gray-900 dark:text-white mb-4 capitalize">
                    {groupName}
                </h2>
                <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
                    for _, app := range apps {
                        @ApplicationCard(app)
                    }
                </div>
            </div>
        }
    }
}

templ ApplicationCard(app *model.Application) {
    <a href={app.URL} class="block group">
        <div class="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-6 hover:shadow-lg transition-shadow h-32">
            <div class="flex items-start space-x-4 h-full">
                <div class="flex-shrink-0">
                    <div class="h-12 w-12 bg-gray-100 dark:bg-gray-700 rounded-lg flex items-center justify-center">
                        <span class="text-2xl">{app.Icon}</span>
                    </div>
                </div>
                <div class="flex-1 min-w-0 flex flex-col">
                    <h3 class="text-lg font-semibold text-gray-900 dark:text-white group-hover:text-blue-600 dark:group-hover:text-blue-400">
                        {app.Name}
                    </h3>
                    <p class="mt-1 text-sm text-gray-600 dark:text-gray-400 line-clamp-2">
                        {app.Description}
                    </p>
                </div>
            </div>
        </div>
    </a>
}
```

---

### Migration Strategy

1. **Database Setup:**
   - Create tables for applications and attribute_definitions
   - Seed initial attribute definitions
   - Migrate existing mock data to database

2. **Backward Compatibility:**
   - Keep mock data in code as fallback
   - Feature flag for database vs. mock data
   - Graceful degradation if database unavailable

3. **Incremental Rollout:**
   - Phase 2a: Read-only from database
   - Phase 2b: Add application management UI (CRUD)
   - Phase 2c: Dynamic attribute definition management

---

### Future Enhancements

#### Admin UI for Attribute Management
- [ ] Add/edit/delete attribute definitions
- [ ] Reorder attributes (sort_order)
- [ ] Add new valid values to existing attributes
- [ ] Set default filters

#### Application Management
- [ ] Admin UI for adding/editing applications
- [ ] Bulk import from CSV/JSON
- [ ] Application health checks
- [ ] Usage analytics per application

#### Advanced Filtering
- [ ] Multiple filter values (OR logic)
- [ ] Search/filter by app name
- [ ] Favorite/pin applications
- [ ] Recently used applications

#### ABAC Integration (with Keycloak)
- [ ] Filter apps based on user roles/attributes
- [ ] Hide apps user doesn't have access to
- [ ] Show "Request Access" for restricted apps
- [ ] Audit log for app access attempts

---

### Testing Checklist

- [ ] Unit tests for attribute matching logic (wildcard handling)
- [ ] Unit tests for repository layer (mocked DB)
- [ ] Unit tests for service layer (grouping, validation)
- [ ] Integration tests for filtering endpoints
- [ ] E2E tests for filter UI interactions
- [ ] Test with empty attribute values
- [ ] Test with invalid attribute values
- [ ] Test grouping with missing grouping attribute
- [ ] Performance test with large number of apps (1000+)

---

### Performance Considerations

1. **Database Queries:**
   - GIN index on JSONB attributes column
   - Consider materialized view for frequently accessed groupings
   - Cache attribute definitions (rarely change)

2. **Client-Side:**
   - Minimize HTMX swaps (only swap app container, not filters)
   - Debounce filter changes if adding real-time search
   - Consider pagination for 100+ apps

3. **Caching:**
   - Redis cache for application list (5-minute TTL)
   - Cache grouped results per filter combination
   - Invalidate cache on application updates

---

### Security Considerations

- [ ] Validate all filter inputs against attribute schema
- [ ] Prevent SQL injection in attribute queries (use parameterized queries)
- [ ] Rate limit filtering endpoints
- [ ] Audit log for application access
- [ ] CSRF protection on filter requests (if using POST)

---

### Monitoring & Observability

- [ ] Track most-used filters (analytics)
- [ ] Monitor query performance for attribute filtering
- [ ] Alert on high error rates for filter endpoints
- [ ] Dashboard for application usage by space/environment
- [ ] Track empty filter results (might indicate data issues)

---

### Documentation

- [ ] Document attribute schema format
- [ ] API documentation for filter endpoints
- [ ] User guide for filtering applications
- [ ] Admin guide for managing attributes
- [ ] Architecture decision record for attribute system design