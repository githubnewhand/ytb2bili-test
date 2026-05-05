const API_PREFIX = '/api/v1';

function trimTrailingSlash(value: string) {
  return value.replace(/\/+$/, '');
}

function trimApiPrefix(value: string) {
  return trimTrailingSlash(value).replace(/\/api\/v1$/i, '');
}

export function getApiBaseUrl() {
  const configuredBaseUrl = process.env.NEXT_PUBLIC_API_URL?.trim();
  if (configuredBaseUrl) {
    return trimTrailingSlash(configuredBaseUrl);
  }

  return API_PREFIX;
}

export function getApiUrl(path: string) {
  const normalizedPath = path.startsWith('/') ? path : `/${path}`;
  return `${getApiBaseUrl()}${normalizedPath}`;
}

export function resolveApiResourceUrl(resourceUrl: string) {
  if (!resourceUrl || /^https?:\/\//i.test(resourceUrl)) {
    return resourceUrl;
  }

  const configuredBaseUrl = process.env.NEXT_PUBLIC_API_URL?.trim();
  if (!configuredBaseUrl) {
    return resourceUrl;
  }

  try {
    return new URL(resourceUrl, `${trimApiPrefix(configuredBaseUrl)}/`).toString();
  } catch {
    return resourceUrl;
  }
}