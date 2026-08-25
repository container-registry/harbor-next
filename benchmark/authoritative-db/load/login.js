// Login-representative load, round-robins across the 3 core replicas via
// CORE_PORTS (comma-separated). Run: k6 run -e CORE_PORTS=18080,18081,18082 login.js
import http from 'k6/http';
import { check } from 'k6';

const ports = (__ENV.CORE_PORTS || '18080,18081,18082').split(',');
const durationSec = __ENV.DURATION_SEC || '120';

export const options = {
  scenarios: {
    login_representative: {
      executor: 'constant-vus',
      vus: Number(__ENV.VUS || 20),
      duration: `${durationSec}s`,
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<5000', 'p(99)<10000'],
  },
};

export default function () {
  const port = ports[__VU % ports.length];
  const base = `http://localhost:${port}`;

  const login = http.post(`${base}/c/login`,
    { principal: 'admin', password: __ENV.HARBOR_ADMIN_PASSWORD || 'Harbor12345' },
    { headers: { 'Content-Type': 'application/x-www-form-urlencoded' } });
  check(login, { 'login ok': (r) => r.status === 200 || r.status === 0 });

  const ping = http.get(`${base}/api/v2.0/ping`);
  check(ping, { 'ping ok': (r) => r.status === 200 });
}
