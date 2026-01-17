# Cloak Apps

[![Go Version](https://img.shields.io/badge/Go-1.25.0-00ADD8?style=flat-square&logo=go)](https://go.dev/doc/go1.25)
[![Templ](https://img.shields.io/badge/Templ-0.3.943-red?style=flat-square)](https://templ.guide)
[![Tailwind CSS](https://img.shields.io/badge/Tailwind_CSS-3.4.11-38B2AC?style=flat-square&logo=tailwind-css)](https://tailwindcss.com)
[![HTMX](https://img.shields.io/badge/HTMX-2.0.7-purple?style=flat-square)](https://htmx.org)
[![Alpine.js](https://img.shields.io/badge/Alpine.js-3.15.0-2D3441?style=flat-square)](https://alpinejs.dev)

An internal application portal for organization engineers, similar to Okta's app integration dashboard but natively supporting Keycloak SSO. Built as a Hypermedia-Driven Application (HDA) using the GOTH stack.

## Overview

Cloak Apps serves as a centralized hub where users can access all applications they have permission to use. Keycloak acts as the single source of truth for authentication and role-based access control (RBAC).

**Key Features:**
- [x] Keycloak SSO integration for authentication
- [x] Application grouping by space (operations, tools, mvp, etc.)
- [x] Environment filtering (production, development, all)
- [x] Dark/light mode support
- [x] Card and list view modes
- [x] Type-safe templates with Templ
- [x] Hypermedia-driven architecture with HTMX
- [x] OpenTelemetry tracing and Prometheus metrics (`/metrics`)

Built from the [Effiware GOTH template](https://github.com/Effiware/goth-template).

## Quick Start

### Prerequisites

- Go v1.25+
- npm v11.4+
- node v24.4+
- Air v1.63.0 (for hot reload)
- Templ CLI 0.3.943
- GNU Make 3.81 (optional)

Or use Docker 28.1+

---

## Development Setup

### Option 1: Local Machine

1. **Install dependencies**
   ```bash
   make prep
   ```

2. **Build Tailwind and Go**
   ```bash
   make build-local
   ```

3. **Run the application** with Hot Reload using Air 
   ```bash
    make air
    ```

### Option 2: Docker (Recommended)

Either do `make prep` (will also install Go/Node dependencies on the host machine) or copy `.env.example` to `.env` and modify as needed.

1. **Generate certificates**
   ```bash
   make gen-certs
   ```

2. **Build the Docker image**
   ```bash
   make docker-build
   ```

3. **Run the Docker container**
   ```bash
   make docker-up
   ```

4. **Stop the Docker container**
   ```bash
   make docker-down
   ```

### Add hostnames to your local DNS resolver

If you're on Mac or linux simply add below line to your `/etc/hosts`

```txt
127.0.0.1    keycloak
```

### Import sample realm

The app as is uses a realm called **cloak-apps-realm**, the easies way to start using the project is to create a new
realm in Keycloak with the same name and import (seed) the default data from [cloak-apps-realm-export.json](./seed/cloak-apps-realm-export.json)

You should be able to log in to admin console on `https://localhost/admin` - it uses self-signed certificates so you
have to accept potential risk alert in the browser.

Detailed information is available in [KEYCLOAK_CONFIGURATION.md](./KEYCLOAK_CONFIGURATION.md)

---

## Access the Application

In `.env` file there are port overwrites with the default setup. You shouldn't need to change them but there is always
a possibility to do so.

Docker Compose includes Keycloak, Jaeger and Prometheus. Access them at `https://localhost`, `http://localhost:8083`
and `http://localhost:8084` respectively

Open your browser and navigate to `http://localhost:<app-port>` (default is 8080).

---

## Documentation

- [CLAUDE.md](CLAUDE.md) - Project architecture and design decisions
- [TODO.md](TODO.md) - Future implementation phases and roadmap
