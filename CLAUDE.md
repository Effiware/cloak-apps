# Cloak Apps - Internal Application Portal

## Project Overview

Cloak Apps is an internal web application portal for organization engineers, similar to Okta's app integration dashboard but natively supporting Keycloak SSO instead of Okta. The application serves as a centralized hub where users can see thumbnails and access links to all applications they have permission to use.

### Key Characteristics

- **Single Source of Truth**: Keycloak acts as the authentication and authorization provider
- **RBAC Integration**: All role-based access control is handled in Keycloak
  - User-to-application assignments
  - Permissions embedded in tokens
- **Application Portal**: Displays available applications based on user permissions
- **Internal Tool**: Designed for engineers within the organization

## Tech Stack (GOTH)

The application uses the GOTH stack:

- **G**o + Chi - Backend server and routing
- **T**empl - Type-safe templating for Go
- **H**TML - Semantic markup
- **T**ailwind - Utility-first CSS framework

### Additional Technologies

- **HTMX** - For hypermedia-driven interactivity
- **Keycloak** - Identity and access management (SSO + RBAC)

## Architecture: Hypermedia-Driven Application (HDA)

This project follows the Hypermedia-Driven Application pattern, which combines:
- The simplicity and flexibility of traditional Multi-Page Applications (MPAs)
- The enhanced user experience of Single-Page Applications (SPAs)

### Two Core HDA Constraints

1. **Declarative HTML Syntax**: Uses HTML-embedded declarative syntax (HTMX attributes) rather than imperative JavaScript for interactivity
2. **Hypermedia Communication**: Server communicates with clients using hypermedia (HTML) instead of JSON APIs

### HDA Principles

- **REST Fidelity**: Maintains HATEOAS (Hypermedia As The Engine of Application State)
- **Server Responsibility**: Server manages application state and UI generation
- **JavaScript as Enhancement**: JavaScript augments the experience rather than driving the entire application logic
- **Progressive Enhancement**: Core functionality works without JavaScript

### Why HDA?

- Reduces complexity compared to SPA architectures
- Maintains modern user experience through HTMX
- Server-side rendering with dynamic interactivity
- Type-safe components with Templ
- No complex client-side state management needed

## Project Structure (Onion Model)

Following Templ's recommended onion architecture:

### Core Layers

1. **HTTP Handlers** - Process requests and coordinate responses using components
2. **Services** - Execute application logic without knowing about HTTP or HTML details
3. **Database Access** - Manage data operations while isolating the database representation

### Supporting Packages

- `components/` - Templ UI components
- `main.go` - Application entry point and server configuration

### Key Principle

Layering should enhance clarity without fragmenting logic unnecessarily across files. Don't take separation to an extreme level.

## How HTMX Works in This Project

HTMX extends HTML to enable modern browser features through declarative attributes:

### Key HTMX Attributes

- **`hx-get`, `hx-post`, `hx-put`, `hx-patch`, `hx-delete`** - Specify HTTP methods and endpoints on any element
- **`hx-trigger`** - Control when requests fire (clicks, changes, polling, scroll, custom events)
- **`hx-target`** - Direct responses to specific elements using CSS selectors
- **`hx-swap`** - Determine how content integrates (innerHTML, outerHTML, append, etc.)

### HTMX Benefits

- No build tools required
- Ships as a single script tag
- Minimal boilerplate
- Works with server-side HTML responses
- Maintains REST architectural principles

## How Templ Works

Templ is a type-safe templating language for Go:

### Characteristics

- Write HTML components using Go syntax
- Type safety checked at compile time
- Native Go integration (parallelism, standard library)
- No separate template files needed
- Dependency injection through constructor functions

### Integration Pattern

```go
// Templ components are rendered directly within Go code
func Handler(w http.ResponseWriter, r *http.Request) {
    component := components.AppCard(appData)
    component.Render(context.Background(), w)
}
```

## Authentication & Authorization

### Keycloak Integration

- **SSO Provider**: Keycloak handles all authentication
- **Token-Based**: User permissions embedded in tokens
- **RBAC**: All roles and permissions managed in Keycloak
- **Application Assignment**: Users assigned to specific applications in Keycloak

### Security Flow

1. User authenticates with Keycloak
2. Keycloak issues token with permissions
3. Cloak Apps reads token to determine:
   - Which applications user has access to
   - What permissions user has within those applications
4. Portal displays only accessible applications

## Development Guidelines

### General Principles

- Server-side rendering by default
- Use HTMX for dynamic interactions
- Keep JavaScript minimal and progressive
- Maintain type safety through Templ
- Follow REST principles with hypermedia responses

### Code Organization

- Handlers coordinate but contain minimal logic
- Services contain business logic
- Components are reusable Templ elements
- Database layer isolates data access
- Clear dependency injection patterns

### CSS with Tailwind

- Utility-first approach
- Component styles defined in Templ components
- Minimal custom CSS
- Responsive design using Tailwind breakpoints

## Reference Links

- **HDA Concept**: https://htmx.org/essays/hypermedia-driven-applications/
- **Templ Project Structure**: https://templ.guide/project-structure/project-structure
- **HTMX Documentation**: https://htmx.org/docs/
- **Okta App Integrations** (concept reference): https://www.okta.com/resources/whitepaper/app-integrations/

## Current Status

Project is in skeleton setup phase. Core infrastructure and architectural decisions are being established.