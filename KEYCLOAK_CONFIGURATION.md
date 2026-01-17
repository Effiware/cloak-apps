# Keycloak Configuration Guide

This guide covers the required Keycloak configuration for the Cloak Apps portal to function correctly.

## Prerequisites

- Keycloak realm created (e.g., `cloak-apps-realm`)
- Portal client created (e.g., `cloak-apps-portal`)

## Required Configuration

### 1. Enable Client Roles in ID Token

**Why**: The application reads client roles from the `resource_access` claim in the ID token to determine which applications a user can access.

**Steps**:

1. Go to **Client Scopes** → **roles** → **Mappers** tab
2. Click **Add mapper** → **By configuration** → Select **User Client Role**
3. Configure the mapper:
   - **Name**: `client-roles`
   - **Multivalued**: `ON`
   - **Token Claim Name**: `resource_access.${client_id}.roles`
   - **Claim JSON Type**: `String`
   - **Add to ID token**: `ON` ✅ **CRITICAL**
   - **Add to access token**: `ON`
   - **Add to userinfo**: `ON`
4. Click **Save**

5. Verify the scope is assigned:
   - Go to **Clients** → `cloak-apps-portal` → **Client Scopes** tab
   - Ensure `roles` is in **Assigned Default Client Scopes**

### 2. Create Application Clients

For each application you want to display in the portal:

1. Go to **Clients** → **Create client**
2. Configure:
   - **Client ID**: Unique identifier (e.g., `grafana`, `prometheus`)
   - **Name**: Display name shown in portal
   - **Description**: JSON metadata (see below)
   - **Root URL / Home URL**: Application URL
   - **Logo URL** (in Attributes → Advanced): Thumbnail image URL

3. **Description Field Format** (JSON):
   ```json
   {
     "text": "Human-readable description",
     "tooltip": "Optional tooltip text",
     "icon_emoji": "📊",
     "sso_enabled": true,
     "tags": ["monitoring", "observability"],
     "order": 1
   }
   ```

4. Add **Client Scopes** for categorization:
   - **Space**: `space-operations`, `space-tools`, `space-core`
   - **Environment**: `env-production`, `env-development`, `env-shared`

   Go to **Client** → **Client Scopes** tab → Add to **Optional Client Scopes**

### 3. Create Client Roles

For each application client:

1. Go to **Clients** → Select client → **Roles** tab
2. Click **Create role**
3. Add roles like: `user`, `admin`, `viewer`, etc.
4. Click **Save**

### 4. Assign Users to Applications

To grant a user access to an application:

1. Go to **Users** → Select user → **Role Mappings** tab
2. From **Client Roles** dropdown, select the application client
3. Select one or more roles from **Available Roles**
4. Click **Add selected**

**Important**: Users must have at least one role assigned to a client to see that application in the portal.

## Client Scopes Reference

Create these client scopes in **Client Scopes** section:

### Space Categories
- `space-operations` - Operations/Infrastructure tools
- `space-tools` - General productivity tools
- `space-core` - Core business applications

### Environment Categories
- `env-production` - Production environment
- `env-development` - Development environment
- `env-shared` - Shared across environments

## Troubleshooting

### Issue: User sees 0 applications after login

**Check the logs** (application provides debug output):

```
[DEBUG] User username authenticated - ClientRoles: map[]
[DEBUG] User username has roles in 0 clients
```

**Diagnosis**: `ClientRoles: map[]` means the ID token doesn't contain `resource_access` claim.

**Solution**:
1. Verify the **User Client Role** mapper exists in the `roles` client scope
2. Ensure **Add to ID token** is enabled on the mapper
3. Verify `roles` scope is in the client's **Assigned Default Client Scopes**
4. Log out and log in again to get a new token

---

### Issue: User authenticated but specific applications missing

**Check the logs**:

```
[DEBUG] User username authenticated - ClientRoles: map[app1:[user]]
[DEBUG] Access DENIED for client 'app2' - User has no roles
```

**Diagnosis**: User has roles for some clients but not others.

**Solution**:
1. Go to **Users** → Select user → **Role Mappings**
2. Select the missing client from **Client Roles** dropdown
3. Assign at least one role to the user for that client

---

### Issue: ClientRoles has wrong client IDs

**Check the logs**:

```
[DEBUG] User username authenticated - ClientRoles: map[wrong-id:[user]]
[DEBUG] Access DENIED for client 'grafana' - User has no roles
```

**Diagnosis**: Client IDs in the token don't match actual client IDs in Keycloak.

**Solution**:
1. Verify the client ID in **Clients** section matches exactly (case-sensitive)
2. Check that roles are assigned to the correct client
3. Ensure the user has roles on the actual client, not a different one

---

### Issue: Applications show but SSO redirect fails

**Diagnosis**: Client authentication settings incorrect.

**Solution**:
1. Ensure client **Access Type** is set to `confidential` or `public` as appropriate
2. Verify **Valid Redirect URIs** includes your application callback URL
3. Check **Web Origins** allows your application domain

## Verification Checklist

- [ ] `roles` client scope has **User Client Role** mapper
- [ ] Mapper has **Add to ID token** enabled
- [ ] `roles` scope assigned to `cloak-apps-portal` client
- [ ] Application clients created with proper metadata
- [ ] Client roles created for each application
- [ ] Users assigned roles to specific application clients
- [ ] Client scopes (space-*, env-*) created and assigned

## Testing the Configuration

1. Log in to the portal
2. Check application logs for debug output:
   - Should see `ClientRoles: map[app1:[role1] app2:[role2]]`
   - Should see `Access GRANTED` for applications with assigned roles
3. Verify applications display in the portal UI
4. Test SSO redirect by clicking on an SSO-enabled application