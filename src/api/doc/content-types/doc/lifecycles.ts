const normalizeAssetUrls = (value: unknown): unknown => {
  if (typeof value !== 'string' || value.length === 0) {
    return value;
  }

  const baseUrl = process.env.S3_BASE_URL?.trim();
  if (!baseUrl || !baseUrl.startsWith('/')) {
    return value;
  }

  const escapedBase = baseUrl.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const absoluteBasePattern = new RegExp(`https?:\\/\\/[^"'\\s)]+${escapedBase}`, 'g');

  return value.replace(absoluteBasePattern, baseUrl);
};

const normalizeDocContent = (event: { params?: { data?: Record<string, unknown> } }) => {
  const data = event.params?.data;
  if (!data || !('content' in data)) {
    return;
  }

  data.content = normalizeAssetUrls(data.content);
};

export default {
  beforeCreate(event: { params?: { data?: Record<string, unknown> } }) {
    normalizeDocContent(event);
  },

  beforeUpdate(event: { params?: { data?: Record<string, unknown> } }) {
    normalizeDocContent(event);
  },
};
