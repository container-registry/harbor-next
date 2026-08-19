# Harbor Portal - Frontend Development Guide

## Overview

The Harbor Portal is an Angular 16 single-page application (SPA) that provides the web-based user interface for Harbor. It uses the Clarity Design System from VMware for UI components.

## Technology Stack

- **Framework**: Angular 16
- **UI Components**: Clarity Design System (`@clr/angular`, `@clr/ui`)
- **Language**: TypeScript
- **Styling**: SCSS
- **Testing**: Karma/Jasmine for unit tests, Cypress for E2E
- **Build**: Angular CLI
- **API Client**: Auto-generated via ng-swagger-gen from OpenAPI spec

## Directory Structure

```
src/portal/
├── src/
│   ├── app/                    # Main application code
│   │   ├── base/               # Base layout and main feature modules
│   │   │   ├── left-side-nav/  # Left navigation and system-level features
│   │   │   │   ├── federated-idps/        # Federated Identity Providers management
│   │   │   │   ├── system-robot-accounts/ # System-level robot accounts
│   │   │   │   ├── config/                # System configuration
│   │   │   │   ├── user/                  # User management
│   │   │   │   ├── group/                 # Group management
│   │   │   │   └── ...
│   │   │   ├── project/        # Project-level features
│   │   │   │   ├── robot-account/         # Project robot accounts
│   │   │   │   ├── repository/            # Repository management
│   │   │   │   ├── tag-feature-integration/ # Tag operations
│   │   │   │   └── ...
│   │   │   └── global-confirmation-dialog/
│   │   ├── shared/             # Shared components, services, and utilities
│   │   │   ├── components/     # Reusable UI components
│   │   │   │   ├── view-token/            # Token/secret display modal
│   │   │   │   ├── inline-alert/          # Inline alert component
│   │   │   │   ├── filter/                # Search/filter component
│   │   │   │   └── ...
│   │   │   ├── services/       # Shared services
│   │   │   ├── units/          # Utility functions
│   │   │   └── entities/       # Shared constants and interfaces
│   │   └── services/           # Application-level services
│   └── i18n/                   # Internationalization files
│       └── lang/
│           ├── en-us-lang.json # English translations
│           └── zh-cn-lang.json # Chinese translations
├── ng-swagger-gen/             # Auto-generated API client code
│   ├── models/                 # TypeScript interfaces from OpenAPI
│   └── services/               # API service classes
└── ...
```

## Key Concepts

### API Client Generation

The API client is auto-generated from the OpenAPI spec (`api/v2.0/swagger.yaml`):

```bash
# Regenerate API client after swagger.yaml changes
task build:gen-apis
```

Generated files are in `ng-swagger-gen/`:
- `models/` - TypeScript interfaces matching API schemas
- `services/` - Angular services for each API endpoint

### Component Patterns

#### Wizard Components
Multi-step forms use Clarity's `clr-wizard`:
```typescript
@ViewChild('wizard') wizard: ClrWizard;

// Reset wizard on cancel
cancel() {
    this.wizard.reset();
    this.reset();
    this.modalOpened = false;
}
```

#### Modal Components
Modal dialogs use Clarity's `clr-modal`:
```html
<clr-modal [(clrModalOpen)]="modalOpened" [clrModalStaticBackdrop]="true">
    <div class="modal-title">Title</div>
    <div class="modal-body">Content</div>
    <div class="modal-footer">Actions</div>
</clr-modal>
```

#### Form Validation
Use Angular reactive forms or template-driven forms with Clarity styling:
```html
<clr-input-container>
    <input clrInput [(ngModel)]="value" name="fieldName" required />
    <clr-control-error *ngIf="...">Error message</clr-control-error>
</clr-input-container>
```

### Translations

All user-facing text should use translation keys:
```html
{{ 'TRANSLATION.KEY' | translate }}
```

Translation files are in `src/i18n/lang/`:
- Add new keys to `en-us-lang.json` (and other language files)
- Keys are organized hierarchically (e.g., `SYSTEM_ROBOT.CREATE_ROBOT`)

## Development Commands

```bash
# Start development server with hot reload
task dev:frontend
# Or directly:
cd src/portal && npm start

# Run linting
npm run lint

# Auto-fix lint issues
npm run lint_fix

# Run unit tests
npm run test

# Build for production
npm run build
```

## Verification After UI Changes (IMPORTANT FOR AI AGENTS)

**After making any UI changes, AI agents SHOULD use subagents to run verification commands in parallel:**

```bash
# Run all three verification commands:
bun run lint_fix        # TypeScript/Angular linting
bun run lint_fix:style  # SCSS/CSS style linting
bun run test            # Unit tests (use --browsers=ChromeHeadless for headless)
```

**Important Notes:**
- **Use subagents** to run these commands for better parallelization and context management
- **Do not commit `console.log`** — the repo's ESLint config (`no-console`) allows only `console.warn` and `console.error`; remove temporary `console.log` calls before refreshing a patch
- Run tests with `--include='**/component-name.spec.ts'` to run specific component tests
- The `lint_fix` commands will auto-fix what they can; remaining errors need manual fixes

## Common Development Tasks

### Adding a New Feature Component

1. Create component in appropriate directory under `src/app/base/`
2. Add to module declarations
3. Add routing if needed
4. Add translation keys to `en-us-lang.json`

### Modifying API Calls

1. If API changed, run `task build:gen-apis` to regenerate client
2. Import service from `ng-swagger-gen/services`
3. Inject in constructor and use

### Adding Validation

```typescript
// In component class
isValidFinalState(): boolean {
    // Clear previous errors
    this.items.forEach(item => (item.error = ''));

    // Validate each item
    for (const item of this.items) {
        if (!this.isValidItem(item)) {
            item.error = 'Error message';
            return false;
        }
    }
    return true;
}
```

### Showing Errors

```typescript
// Inline alert (in modal/wizard)
@ViewChild(InlineAlertComponent) inlineAlert: InlineAlertComponent;
this.inlineAlert.showInlineError(error);

// Global message
this.msgHandler.showError('Error message');
this.msgHandler.showSuccess('Success message');
```

## Federated Identity Providers (FedIdP)

### Overview

Federated Identity Providers allow Harbor to accept JWT tokens from external identity providers (e.g., Kubernetes API server, cloud IdPs) for workload identity federation. This eliminates the need for long-lived robot account secrets.

### Key Components

#### `federated-idps/` - IDP Management
- `list-federated-idps/` - List and manage federated IDPs
- `create-edit-idp/` - Create/edit IDP wizard with OIDC configuration

#### System Robot Accounts with Federation
Located in `system-robot-accounts/`:
- `new-robot/new-robot.component.ts` - Create/edit robot accounts
  - `useFederatedRobot` flag enables federated mode
  - `idpSelection` - Selected federated IDP
  - `claims` - Robot-specific JWT claim rules
  - `inheritedClaims` - IDP-level claims (read-only)

### Claim Rules

Claims are rules that map JWT token claims to robot accounts:

```typescript
interface Claim {
    path: string;   // JWT claim path (e.g., "sub", "kubernetes.io/serviceaccount/namespace")
    value: string;  // Expected value
    error?: string; // Validation error message
}
```

#### Claim Validation Rules

1. **Non-empty**: At least one claim with path and value
2. **Valid format**: Path matches pattern `^[a-zA-Z_][a-zA-Z0-9_\-]*(\.[a-zA-Z_][a-zA-Z0-9_\-]*|\[\d+\]|\[\*\])*$`
3. **No duplicates with inherited claims**: Robot claims cannot override IDP-level claims
4. **No duplicate paths within robot claims**: Each path must be unique
5. **Unique claim set**: No two robots can have identical claim sets (prevents ambiguous authentication)

#### Duplicate Claim Set Detection

When creating/editing a federated robot, the system checks that the claim set is unique:

```typescript
// Build existing robot claim sets when IDP is selected
buildExistingRobotClaimSets(claimRules: any[]) {
    // Groups claims by robot_id as normalized "path:value" strings
}

// Check for duplicates during validation
findDuplicateClaimSet(): number | null {
    // Returns robot_id if duplicate found, null otherwise
}
```

### API Services

```typescript
// FederatedIdpService (ng-swagger-gen/services/federated-idp.service.ts)
ListFederatedIdps({})           // List all IDPs
CreateFederatedIdp({ idp })     // Create new IDP
GetFederatedIdp({ id })         // Get IDP by ID
UpdateFederatedIdp({ id, idp }) // Update IDP
DeleteFederatedIdp({ id })      // Delete IDP
ListClaimRules({ id })          // List claims for an IDP
CreateClaimRules({ id, claims }) // Create claim rules
DeleteClaimRules({ id, claims }) // Delete claim rules
```

### View Token Component

`shared/components/view-token/` displays robot credentials:
- For regular robots: Shows name and secret
- For federated robots: Shows name, inherited claims, and robot-specific claims

```typescript
// Fetch claims when robot is set
set robot(value: Robot) {
    this._robot = value;
    if (value?.federatedidp_id > 0) {
        this.fetchClaimsForRobot(value.federatedidp_id, value.id);
    }
}
```

## Debugging Tips

### Console Logging

Development logging is available (remove before production):
```typescript
console.log('[componentName] Debug info:', data);
```

### Common Issues

1. **Robot ID undefined in callbacks**: Ensure async operations complete before accessing robot properties
2. **Claims not filtering correctly**: Check `robot_id` type (number vs string) - use `+value` for conversion
3. **Form validation not updating**: Call validation method on `(ngModelChange)` or `(blur)` events
