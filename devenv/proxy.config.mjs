// Proxy config for Docker dev environment
// Routes API calls to the core container
export default [
  {
    context: [
      "/api",
      "/c",
      "/i18n",
      "/chartrepo",
      "/LICENSE",
      "/swagger.json",
      "/swagger2.json",
      "/devcenter-api-2.0",
      "/swagger-ui.bundle.js"
    ],
    target: "http://core:8080",
    secure: false,
    changeOrigin: true,
    logLevel: "debug"
  }
];
