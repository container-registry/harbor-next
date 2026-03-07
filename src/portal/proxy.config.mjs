import HttpsProxyAgent from 'https-proxy-agent';

const apiProxyTarget = process.env.HARBOR_PROXY_TARGET || 'http://localhost:8080';
const useAgent = process.env.HARBOR_USE_PROXY_AGENT === 'true';
const specifiedAgentServer = process.env.HARBOR_PROXY_AGENT_SERVER || '';
const logLevel = process.env.HARBOR_PROXY_LOG_LEVEL || 'debug';

const HarborProxyConfig = [
  {
    context: [
      '/api',
      '/service',
      '/v2',
      '/chartrepo',
      '/c',
      '/LICENSE',
    ],
    target: apiProxyTarget,
    secure: false,
    changeOrigin: true,
    logLevel,
  },
];

function setupForCorporateProxy(proxyConfig) {
  if (useAgent) {
    const agentServer =
      process.env.http_proxy || process.env.HTTP_PROXY || specifiedAgentServer;
    if (agentServer) {
      const agent = new HttpsProxyAgent(agentServer);
      proxyConfig.forEach(entry => {
        entry.agent = agent;
      });
    }
  }
  return proxyConfig;
}

export default setupForCorporateProxy(HarborProxyConfig);
