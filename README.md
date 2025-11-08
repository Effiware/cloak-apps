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
   make build
   ```

3. **Run the application** with Hot Reload using Air 
   ```bash
    make air
    ```

### Option 2: Docker (Recommended)

Either do `make prep` (will also install Go/Node dependencies on the host machine) or copy `.env.example` to `.env` and modify as needed.

1. **Build the Docker image**
   ```bash
   make docker-build
   ```

2. **Run the Docker container**
   ```bash
   make docker-up
   ```

3. **Stop the Docker container**
   ```bash
   make docker-down
   ```

---

## Access the Application

Open your browser and navigate to `http://localhost:<app-port>` (default is 8080).

The port should match the `APP_PORT` in your `.env` file.

---

## Documentation

- [CLAUDE.md](CLAUDE.md) - Project architecture and design decisions
- [TODO.md](TODO.md) - Future implementation phases and roadmap
