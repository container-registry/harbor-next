Swagger UI
============
This is the project based on Swagger UI and Webpack.



Start
============
1. bun install
2. set `HARBOR_PROXY_TARGET` to an available Harbor API target if you are not using the default `http://localhost:8080`
3. bun run start

`task dev:up OPENAPI_UI=true` builds this app inside the portal dev container and serves the generated assets from the portal.
