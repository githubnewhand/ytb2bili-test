const { execSync } = require('child_process');

function getAppVersion() {
  if (process.env.NEXT_PUBLIC_APP_VERSION) {
    return process.env.NEXT_PUBLIC_APP_VERSION;
  }
  try {
    const count = execSync('git rev-list --count HEAD', {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
    }).trim();
    const parsed = Number.parseInt(count, 10);
    return Number.isFinite(parsed) && parsed > 0 ? `0.${parsed}` : '0.1';
  } catch {
    return '0.1';
  }
}

/** @type {import('next').NextConfig} */
const nextConfig = {
  env: {
    BACKEND_URL: process.env.BACKEND_URL || 'http://localhost:8096',
    NEXT_PUBLIC_APP_VERSION: getAppVersion(),
  },
  output: 'export',
  trailingSlash: true,
  distDir: 'out',
  images: {
    unoptimized: true,
    remotePatterns: [
      {
        protocol: 'https',
        hostname: 'passport.bilibili.com',
        pathname: '/x/passport-tv-login/h5/qrcode/auth/**',
      },
    ],
  },
};

module.exports = nextConfig;
