# Dashboard

The Fleet Server dashboard is a modern single-page application built with React, TypeScript, and Tailwind CSS. It provides a comprehensive interface for fleet management, investigation, and incident response.

## Technology Stack

| Technology | Purpose |
|------------|---------|
| **React 18** | UI framework |
| **TypeScript** | Type-safe development |
| **Vite** | Build tool and dev server |
| **Tailwind CSS** | Utility-first styling |
| **React Query** | Data fetching, caching, and synchronization |
| **React Router** | Client-side routing |
| **React Flow** | Process tree graph visualization |
| **Recharts** | Charts and graphs |
| **Lucide React** | Icon library |
| **dagre / ELK** | Graph layout algorithms |
| **JSZip** | ZIP file handling for rule downloads |

## Theme

### Dark Mode (Default)

The dashboard defaults to dark mode with a custom color scheme:
- **Background**: Deep slate (`slate-900`)
- **Sidebar**: Gradient dark background
- **Cards**: Subtle glass effect
- **Accent**: Fibratus blue (`#4c6ef5`)
- **Cursor glow**: Electric blue grid spotlight effect

### Light Mode

Full light mode support:
- Clean white backgrounds
- Blue accent colors (cobalt, not cyan)
- All components styled for both modes
- Blue hover effects on rows and buttons

### Theme Toggle

Located in the sidebar footer:
- Sun/Moon icon toggle
- Persisted to localStorage
- Applied via Tailwind CSS class-based dark mode

## Pages

### Overview

Dashboard home page with:
- **Stat cards** — total agents, active detections, events ingested, rules loaded
- **Detection timeline** — bar chart showing detections over time
- **Severity breakdown** — donut/pie chart of detection severities
- **Recent detections** — table of latest detection alerts
- **Quick actions** — shortcuts to common tasks
- Gradient stat cards with trend indicators
- Clickable detection entries link to detail view

### Agents

Fleet-wide agent management:
- Agent list with status, hostname, OS, version, IP, last heartbeat
- Search and column sorting
- Click to open agent detail page
- Delete agents with auto-uninstall
- Cross-org view with Organization column

### Agent Detail

Comprehensive single-agent view with 15+ sections. See [Agent Management](fleet/agents.md) for full details.

### Detections

Centralized detection/alert management:
- Detection list with severity badges, rule name, agent, timestamp
- Search and filtering
- Slide-out detail panel with full event data
- Process tree navigation
- URL-based state for deep linking

### Events

SIEM-style telemetry investigation:
- Fibratus QL query bar with autocomplete
- Dynamic fields sidebar
- Time range filtering
- Event table with inline previews
- Expandable event detail
- Live streaming mode
- Filter pills

### Rules

Detection rule management:
- Tabbed interface (All, Official, SIGMA, Custom, GitHub, Tuning)
- YAML editor with validation
- GitHub sync configuration
- SigmaHQ integration toggle
- Bulk upload (files, folders, ZIP)
- Download all as ZIP
- Source and severity badges
- Enable/disable toggles

### Macros

Filter macro management:
- List and expression macros
- Create, edit, delete operations
- Used in rule conditions
- Synced to agents

### Audit Log

Security event trail:
- All user actions logged
- Searchable and filterable
- Timestamp, user, action, details
- Root user actions excluded

### Management

Tabbed management console with 8 tabs:

| Tab | Description |
|-----|-------------|
| **Account** | Account settings, tamper protection, isolation whitelist |
| **Organizations** | Org CRUD, org-specific settings |
| **Users** | User management with role and group assignment |
| **Groups** | Permission group management with 60-permission matrix |
| **Enrollment** | Enrollment token management, install commands |
| **Rules** | GitHub sync configuration, account-wide rules |
| **Telemetry** | Retention configuration |
| **Audit** | Detailed audit log view |

### Profile

User self-service:
- Change password
- Update name and email
- View group membership
- View effective permissions

### Super Admin

Root-only administration panel:
- **Accounts** — create/edit/delete accounts with all settings
- **Users** — create users including root users across accounts
- **Database** — interactive database browser (phpMyAdmin-style) with bulk delete
- **System** — server-wide settings, telemetry retention per account
- Purple styling to distinguish from regular admin

### Process Tree

Dedicated full-screen process tree visualization. See [Process Tree](fleet/process-tree.md) for details.

## Components

### Layout

The main layout component provides:
- Fixed sidebar with navigation
- Account switcher (root users)
- Organization switcher
- User profile and sign-out
- Theme toggle
- Permission-based nav item filtering

### Slide Panel

Reusable slide-out panel for detail views:
- Slides in from the right
- Used for detection details, rule editing, user editing
- Close button and click-outside to dismiss

### Confirm Dialog

Confirmation dialog for destructive actions:
- Delete, kill, isolate, uninstall confirmations
- Customizable title, message, and button text
- Prevents accidental destructive operations

### Cursor Glow

Visual effect component:
- Canvas-based grid spotlight following the cursor
- Electric blue glow in dark mode
- Cobalt glow in light mode
- Only renders in dark mode for performance

### Status Badges

Color-coded status indicators:
- **Online**: Green
- **Offline**: Gray
- **Isolated**: Amber
- **Severity**: Critical (red), High (orange), Medium (yellow), Low (blue)

## Data Fetching

### React Query

All data fetching uses TanStack React Query:
- Automatic caching (30s stale time)
- Background refetching
- Optimistic updates for mutations
- Loading and error states
- Query invalidation on mutations

### API Client

Centralized API client (`lib/api.ts`):
- Base URL: `/api/v1`
- Automatic JWT token injection
- Org-scoped endpoints
- 401 redirect to login
- Error response parsing

## Responsive Design

- Sidebar navigation with organization context
- Full-width content area
- Responsive tables with horizontal scroll
- Mobile-friendly form layouts

## Build and Development

```bash
# Development (with API proxy to localhost:8443)
npm run dev

# Production build
npm run build

# Preview production build
npm run preview
```

The Vite dev server proxies `/api` requests to the Go backend on port 8443.
