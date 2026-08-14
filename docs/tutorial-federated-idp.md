# Federated Identity Provider Tutorial

Federated Identity Providers let workloads authenticate to Harbor using short-lived JWT tokens from external identity providers (GitHub Actions, GitLab CI, Kubernetes) instead of static robot secrets. No more managing long-lived credentials.

> **Architecture details**: See [ADR-federated-idp.md](./ADR-federated-idp.md) for complete technical documentation.

---

## Table of Contents

- [Phase 1: Enable Project-Level Identity Providers](#phase-1-enable-project-level-identity-providers)
- [Phase 2: Create System-Level Identity Provider](#phase-2-create-system-level-identity-provider)
- [Phase 2B: Create Robot Account with Federated IdP](#phase-2b-create-robot-account-with-federated-idp)
- [Phase 3: Create Project-Level Identity Provider](#phase-3-create-project-level-identity-provider)
- [Phase 3B: Create Project Robot with Federated IdP](#phase-3b-create-project-robot-with-federated-idp)
- [Manual Mode Configuration (Air-Gapped / Offline)](#manual-mode-configuration-air-gapped--offline)
- [Using JWT Authentication](#using-jwt-authentication)

---

## Phase 1: Enable Project-Level Identity Providers

By default, only system-level Identity Providers are available. Enable this setting to allow project admins to create project-scoped Identity Providers.

> **Note**: Skip this phase if you only need system-level Identity Providers.

### Step 1: Click on Configuration

Click **Configuration** in the left sidebar under Administration.

![Click on Configuration](./img/fedidp-commercial-1.png)

### Step 2: Click on Commercial

Click the **Commercial** tab.

![Click on Commercial](./img/fedidp-commercial-2.png)

### Step 3: Enable Identity Providers for Projects

Click the checkbox next to **Enable Identity Providers for Projects**.

![Enable checkbox](./img/fedidp-commercial-3.png)

### Step 4: Click SAVE

Click **SAVE** to apply the configuration.

![Click SAVE](./img/fedidp-commercial-4.png)

### Step 5: Confirm Success

A success message confirms the configuration was saved.

![Success message](./img/fedidp-commercial-5.png)

---

## Phase 2: Create System-Level Identity Provider

System-level Identity Providers are available to all projects. Only system administrators can create them.

### Step 1: Click NEW IDENTITY PROVIDER

Navigate to **Administration** → **Identity Providers** and click **+ NEW IDENTITY PROVIDER**.

![Click NEW IDENTITY PROVIDER](./img/fedidp-system-1-new-idp.png)

### Step 2: Click on Name Field

Click the **Name** field to enter the identity provider name.

![Click Name field](./img/fedidp-system-2-name-field.png)

### Step 3: Enter Name

Type the name for your identity provider (e.g., `github`).

![Enter name](./img/fedidp-system-3-name-entered.png)

### Step 4: Enter Description

Click the **Description** field and enter a description (e.g., `idp for github actions`).

![Enter description](./img/fedidp-system-4-description.png)

### Step 5: Enter Discovery URL

Enter the **Discovery URL** for your identity provider. For GitHub Actions:
`https://token.actions.githubusercontent.com/.well-known/openid-configuration`

![Enter Discovery URL](./img/fedidp-system-5-discovery-url.png)

### Step 6: Add Claims

Scroll down to see the auto-populated fields (OIDC Configuration, JWKS, Supported Claims). Click **+ ADD MORE** to add IdP-level claims.

![Add claims](./img/fedidp-system-6-add-claims.png)

### Step 7: Enter Claim Path

Click the **Claim Path** field to enter the claim name.

![Enter claim path](./img/fedidp-system-7-claim-path.png)

### Step 8: Type Claim Name

Enter the claim path (e.g., `ref`).

![Type claim name](./img/fedidp-system-8-claim-ref.png)

### Step 9: Enter Claim Value

Click the **Claim Value** field and enter the expected value (e.g., `yourepo`).

![Enter claim value](./img/fedidp-system-9-claim-value.png)

### Step 10: Click OK

Click **OK** to create the identity provider.

![Click OK](./img/fedidp-system-10-click-ok.png)

### Step 11: Confirm Success

A success message confirms the identity provider was created.

![Success message](./img/fedidp-system-11-success.png)

---

## Phase 2B: Create Robot Account with Federated IdP

After creating the Identity Provider, create a robot account that uses it.

> **Note**: System-level robots can use system-level Identity Providers. These IdPs are also available to all projects for creating project robots.

### Step 1: Click Robot Accounts

Navigate to **Administration** → **Robot Accounts**.

![Click Robot Accounts](./img/fedidp-robot-1-click-robot-accounts.png)

### Step 2: Click NEW ROBOT ACCOUNT

Click **+ NEW ROBOT ACCOUNT**.

![Click NEW ROBOT ACCOUNT](./img/fedidp-robot-2-new-robot.png)

### Step 3: Click Name Field

Click the **Name** field.

![Click Name field](./img/fedidp-robot-3-name-field.png)

### Step 4: Enter Robot Name

Type the robot name (e.g., `roboname`).

![Enter robot name](./img/fedidp-robot-4-name-entered.png)

### Step 5: Enter Description

Enter a description for the robot account.

![Enter description](./img/fedidp-robot-5-description.png)

### Step 6: Enable Federated Robot Account

Check **Use federated robot account** checkbox.

![Enable federated checkbox](./img/fedidp-robot-6-federated-checkbox.png)

### Step 7: Click NEXT

Click **NEXT** to proceed to Identity Provider selection.

![Click NEXT](./img/fedidp-robot-7-click-next.png)

### Step 8: Click Identity Provider Dropdown

Click the **Select Identity Provider** dropdown.

![Click IdP dropdown](./img/fedidp-robot-8-select-idp.png)

### Step 9: Select Identity Provider

Select your identity provider (e.g., `github`).

![Select github](./img/fedidp-robot-9-select-github.png)

### Step 10: View Inherited Claims

After selecting the IdP, you'll see **Inherited Claims** from the IdP. Click **Claim Path** to add robot-specific claims.

![View inherited claims](./img/fedidp-robot-10-inherited-claims.png)

### Step 11: Enter Robot Claim Path

Enter the claim path for robot-specific matching (e.g., `sub`).

> **Mandatory**: Every federated robot must have at least one custom claim. This ensures Harbor can correctly identify and match the robot based on JWT claims.

![Enter claim path](./img/fedidp-robot-11-claim-path.png)

### Step 12: Enter Robot Claim Value

Enter the claim value (e.g., `yoursub`).

![Enter claim value](./img/fedidp-robot-12-claim-value.png)

### Step 13: Click NEXT for Permissions

Click **NEXT** to proceed to system permissions.

![Click NEXT](./img/fedidp-robot-13-next-permissions.png)

### Step 14: Select System Permissions

Select system-level permissions for the robot. Click **NEXT** to continue.

![System permissions](./img/fedidp-robot-14-system-permissions.png)

### Step 15: Select Project Permissions

Click on a project to configure project-specific permissions.

![Project permissions](./img/fedidp-robot-15-project-permissions.png)

### Step 16: Select Pull Permission

Check **Pull** permission for Repository.

![Select Pull](./img/fedidp-robot-16-select-pull.png)

### Step 17: Select Push Permission

Check **Push** permission for Repository if needed.

![Select Push](./img/fedidp-robot-17-select-push.png)

### Step 18: Click FINISH

Click **FINISH** to create the robot account.

![Click FINISH](./img/fedidp-robot-18-click-finish.png)

### Step 19: Robot Created Successfully

The robot account is created. Note the **Inherited Claims** (from IdP) and **Robot Claims** (robot-specific).

> **Important**: For federated robots, there is no secret to copy. Authentication happens via JWT tokens.

![Success](./img/fedidp-robot-19-success.png)

---

## Phase 3: Create Project-Level Identity Provider

Project-level Identity Providers are scoped to a single project. Project administrators can create them.

> **Prerequisite**: Complete [Phase 1](#phase-1-enable-project-level-identity-providers) first.

### Step 1: Navigate to Your Project

From the **Projects** page, click on the project where you want to create the Identity Provider.

![Select project](./img/fedidp-project-1-select-project.png)

### Step 2: Click Identity Providers Tab

Click the **Identity Providers** tab in the project navigation.

![Click Identity Providers tab](./img/fedidp-project-2-click-idp-tab.png)

### Step 3: Click NEW IDENTITY PROVIDER

Click **+ NEW IDENTITY PROVIDER** to open the creation dialog.

![Click NEW IDENTITY PROVIDER](./img/fedidp-project-3-click-new-idp.png)

### Step 4: Click Name Field

Click the **Name** field to enter the identity provider name.

![Click Name field](./img/fedidp-project-4-name-field.png)

### Step 5: Enter Name and Discovery URL

Enter the identity provider name (e.g., `project-git`) and the **Discovery URL**. For GitLab:
`https://gitlab.com/.well-known/openid-configuration`

![Enter name](./img/fedidp-project-5-name-entered.png)

### Step 6: Validate Discovery URL

The system validates the Discovery URL. If invalid, you'll see an error message. Ensure the URL points to a valid OpenID Configuration endpoint.

![Discovery URL validation](./img/fedidp-project-6-discovery-url.png)

### Step 7: Review Auto-Populated Fields

After entering a valid Discovery URL, the form auto-populates with:
- **Issuer**: The identity provider's issuer URL
- **JWKS URI**: URL for JSON Web Key Set
- **OIDC Configuration**: Full OpenID Connect configuration
- **JWKS**: JSON Web Key Set data
- **Supported Claims**: Available claims from the IdP

Click **+ ADD MORE** to add IdP-level claims.

![Populated form](./img/fedidp-project-7-populated-form.png)

### Step 8: Click Claim Path Field

Click the **Claim Path** field to add a new claim.

![Add claim](./img/fedidp-project-8-add-claim.png)

### Step 9: Enter Claim Path

Enter the claim path (e.g., `preferred_username`).

![Enter claim path](./img/fedidp-project-9-claim-path.png)

### Step 10: Enter Claim Value and Click OK

Enter the claim value (e.g., `username`) and click **OK** to create the identity provider.

![Click OK](./img/fedidp-project-10-click-ok.png)

### Step 11: Confirm Success

A success message confirms the identity provider was created. The IdP appears in the list with its issuer, algorithms, and supported claims.

![Success message](./img/fedidp-project-11-success.png)

---

## Phase 3B: Create Project Robot with Federated IdP

After creating Identity Providers, create a robot account within your project that uses federated authentication.

### Identity Provider Scope Rules

Before creating project robots, understand these important scope rules:

| IdP Level | Available To | Use Case |
|-----------|--------------|----------|
| **System-level** | All projects + system robots | Shared IdP for organization-wide access |
| **Project-level** | Only that project's robots | Isolated IdP for project-specific access |

> **Important Rules**:
> - **System IdPs** are available to all projects and system-level robots
> - **Project IdPs** are only usable within that specific project
> - **Identity routing**: IdPs may share an issuer across projects and scopes, but each exact `(issuer, audience)` pair must be globally unique
> - **Mandatory identity claims**: Configure one non-empty IdP-level `aud` claim and the derived `iss` claim. An IdP without an audience cannot authenticate
> - **Mandatory claims**: Every federated robot must have at least one custom claim to enable proper robot matching

### Step 1: Click NEW ROBOT ACCOUNT

Navigate to your project's **Robot Accounts** tab and click **+ NEW ROBOT ACCOUNT**.

![Click NEW ROBOT ACCOUNT](./img/fedidp-project-robot-1-click-new-robot.png)

### Step 2: Click Name Field

Click the **Name** field to enter the robot name.

![Click Name field](./img/fedidp-project-robot-2-name-field.png)

### Step 3: Enter Robot Name

Type the robot name (e.g., `test`).

![Enter robot name](./img/fedidp-project-robot-3-name-entered.png)

### Step 4: Enter Description

Enter an optional description for the robot account.

![Enter description](./img/fedidp-project-robot-4-description.png)

### Step 5: Enable Federated Robot Account

Check the **Use federated robot account** checkbox. This changes the wizard to include an Identity Provider selection step.

![Enable federated checkbox](./img/fedidp-project-robot-8-federated-checkbox.png)

### Step 6: Click NEXT

Notice the wizard now shows 3 steps. Click **NEXT** to proceed.

![Click NEXT](./img/fedidp-project-robot-9-checkbox-enabled.png)

### Step 7: Click Identity Provider Dropdown

Click the **Select Identity Provider** dropdown to see available IdPs.

![Click IdP dropdown](./img/fedidp-project-robot-10-idp-dropdown.png)

### Step 8: Select Identity Provider

The dropdown shows **both system-level and project-level Identity Providers**:
- `github` - System-level IdP
- `k8s-github-actions-test` - System-level IdP
- `project-git` - Project-level IdP (created in Phase 3)

Select your preferred IdP.

![IdP list showing system and project IdPs](./img/fedidp-project-robot-11-idp-list.png)

### Step 9: Review Inherited Claims

After selecting the IdP, you'll see **Inherited Claims** from the IdP configuration. Click the **Claim Path** field to add robot-specific claims.

![IdP selected with inherited claims](./img/fedidp-project-robot-12-idp-selected.png)

### Step 10: Enter Claim Path

Enter a claim path for robot-specific matching (e.g., `ref_path`).

> **Mandatory**: Every federated robot must have at least one custom claim. This ensures Harbor can correctly identify and match the robot based on JWT claims.

![Enter claim path](./img/fedidp-project-robot-13-claim-path.png)

### Step 11: Enter Claim Value

Enter the claim value (e.g., `main` to restrict to main branch).

![Enter claim value](./img/fedidp-project-robot-14-claim-value.png)

### Step 12: Click NEXT for Permissions

Click **NEXT** to proceed to permission selection.

![Click NEXT](./img/fedidp-project-robot-15-click-next.png)

### Step 13: Select Permissions

Select the permissions for this robot. Click **Repository** row to see available permissions.

![Select permissions](./img/fedidp-project-robot-16-permissions.png)

### Step 14: Select Pull Permission

Check the **Pull** permission for Repository access.

![Select Pull](./img/fedidp-project-robot-17-pull-checked.png)

### Step 15: Select Push Permission

Check the **Push** permission if the robot needs write access.

![Select Push](./img/fedidp-project-robot-18-push-checked.png)

### Step 16: Click FINISH

Click **FINISH** to create the robot account.

### Step 17: Robot Created Successfully

The robot is created successfully. The dialog shows:
- **Inherited Claims**: Claims from the IdP (aud, iss, preferred_username)
- **Robot Claims**: Custom claims for this robot (ref_path: main)

> **Note**: For federated robots, there is no secret to copy. Authentication happens via JWT tokens from your IdP.

![Robot created successfully](./img/fedidp-project-robot-19-success.png)

---

## Manual Mode Configuration (Air-Gapped / Offline)

Manual Mode allows you to create Identity Providers without Harbor needing network access to your OIDC provider. This is useful for:

- **Self-hosted environments** where the IdP is on an internal network
- **Air-gapped deployments** with no external network access
- **Kubernetes clusters** not reachable from Harbor
- **Firewall-restricted environments** where OIDC discovery URLs are blocked

### Online vs Manual Mode

| Mode                 | JWKS Validation                         | Best For           | Trade-off                           |
| -------------------- | --------------------------------------- | ------------------ | ----------------------------------- |
| **Online (Default)** | Harbor fetches JWKS from IdP's JWKS URI | Most deployments   | Requires network access to IdP      |
| **Manual**           | Harbor uses locally-stored JWKS         | Air-gapped/offline | Manual JWKS updates on key rotation |

> **Recommendation**: For most deployments, use the default **Online Mode** where Harbor validates JWTs by fetching JWKS from your OIDC provider's JWKS URI. This only requires your OIDC endpoint to be accessible from Harbor and automatically handles key rotations.
>
> Only use Manual Mode when your IdP is truly unreachable from Harbor.

### Step 1: Click NEW IDENTITY PROVIDER

Navigate to Identity Providers and click **+ NEW IDENTITY PROVIDER**.

![Click NEW IDENTITY PROVIDER](./img/fedidp-manual-1-click-new-idp.png)

### Step 2: Enter Name

Enter a name for the identity provider (e.g., `manual`).

![Enter name](./img/fedidp-manual-2-name-field.png)

### Step 3: Enable Manual Mode

Check the **Manual Mode** checkbox. This enables offline configuration.

![Enable Manual Mode](./img/fedidp-manual-3-manual-checkbox.png)

### Step 4: Manual Mode Enabled

With Manual Mode enabled, the **OIDC Configuration** and **JWKS** fields become editable. You'll need to manually provide these values.

![Manual Mode enabled](./img/fedidp-manual-4-checkbox-enabled.png)

### Step 5: Enter OIDC Configuration

Paste your OIDC provider's configuration JSON. You can obtain this from your IdP's `/.well-known/openid-configuration` endpoint (fetched externally).

The form will parse the configuration and populate:
- **Issuer**: The IdP's issuer URL
- **JWKS URI**: Location of the JSON Web Key Set
- **Supported Claims**: Available claims from the IdP

![Enter OIDC Configuration](./img/fedidp-manual-5-oidc-config.png)

### Step 6: Enter JWKS

Paste your OIDC provider's JWKS JSON. You can obtain this from your IdP's JWKS URI endpoint (fetched externally).

![Enter JWKS](./img/fedidp-manual-6-jwks-field.png)

### Step 7: Add Claims

Add IdP-level claims to restrict which JWTs can use this Identity Provider.

![Add claims](./img/fedidp-manual-7-add-claim.png)

### Step 8: Click OK

Click **OK** to create the Identity Provider.

![Click OK](./img/fedidp-manual-8-click-ok.png)

### Step 9: Confirm Success

The Identity Provider is created with **Manual Mode = Yes** shown in the list.

![Success - Manual Mode Yes](./img/fedidp-manual-9-success.png)

### Maintaining Manual Mode IdPs

> **Important**: When you rotate JWT signing keys on your IdP, you must manually update the JWKS in Harbor:
>
> 1. Fetch the new JWKS from your IdP (externally)
> 2. Navigate to Identity Providers in Harbor
> 3. Select your Manual Mode IdP and click **EDIT**
> 4. Update the **JWKS** field with the new JSON
> 5. Click **OK** to save

Failure to update JWKS after key rotation will cause JWT authentication failures.

---

## Using JWT Authentication

Once configured, authenticate using JWT tokens:

### Docker Login

```bash
# Get JWT from your IdP, then pipe it via stdin to avoid
# leaking the token into shell history and process args:
echo "$JWT_TOKEN" | docker login -u jwt --password-stdin harbor.example.com
```

### GitHub Actions

```yaml
permissions:
  id-token: write

steps:
  - name: Get OIDC Token
    run: |
      TOKEN=$(curl -sLS "${ACTIONS_ID_TOKEN_REQUEST_URL}&audience=harbor.example.com" \
        -H "Authorization: Bearer ${ACTIONS_ID_TOKEN_REQUEST_TOKEN}" | jq -r '.value')
      echo "$TOKEN" | docker login harbor.example.com -u jwt --password-stdin
```

### GitLab CI

```yaml
build:
  id_tokens:
    HARBOR_TOKEN:
      aud: https://harbor.example.com
  script:
    - echo "$HARBOR_TOKEN" | docker login harbor.example.com -u jwt --password-stdin
```

---

## Troubleshooting

| Error | Cause | Fix |
|-------|-------|-----|
| `no IdP found for issuer` | Issuer URL mismatch | Check IdP issuer matches JWT `iss` claim exactly |
| `key not found in JWKS` | JWT `kid` not in JWKS | Verify JWKS contains key matching JWT header |
| `no matching robot found` | Claims don't match | Check robot claims match JWT claims |
| `robot is disabled` | Robot account disabled | Enable robot in UI |

> **Debug tip**: Decode JWT at [jwt.io](https://jwt.io) to inspect claims (non-sensitive tokens only).
