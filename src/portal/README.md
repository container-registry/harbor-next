![Harbor UI](https://raw.githubusercontent.com/goharbor/website/master/docs/img/readme/harbor_logo.png)

# Harbor UI

This project is the web interface for [Harbor](https://goharbor.io), built using [Clarity Design System](https://clarity.design/) and Angular.

## Getting Started

### 1. Use the correct Node version

To ensure compatibility with dependencies, use the Node version defined in `.nvmrc`.

```
nvm install   # Install the Node version from .nvmrc (if not already installed)
nvm use       # Switch to the specified Node version
```

### 2. Install dependencies

```
npm install
```

> Note: `npm install` should automatically trigger the `postinstall` script.
If `postinstall` scripts were not triggered, then run manually:  `npm run postinstall`


### 3. Configure proxy targets

`proxy.config.mjs` is tracked and reads its targets from environment variables:

- `HARBOR_PROXY_TARGET` defaults to `http://localhost:8080`
- `OPENAPI_UI=true` builds the OpenAPI UI assets into the portal dev container before `ng serve`
- `HARBOR_USE_PROXY_AGENT=true` enables `https-proxy-agent`
- `HARBOR_PROXY_AGENT_SERVER` overrides the corporate proxy agent URL

When using `task dev:up`, the containerized dev stack sets the proxy target automatically.
OpenAPI UI stays disabled unless you start with `OPENAPI_UI=true`.

### 4. Start the development server

```sh
npm run start
```

To enable the standalone Swagger UI route in dev:

```sh
OPENAPI_UI=true task dev:up
```

### 5. Open the application

Open your browser at http://localhost:4200
