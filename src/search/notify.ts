type LifecycleEvent = {
  result?: {
    docId?: unknown;
  };
};

// Search remains outside Strapi: lifecycle hooks only signal the affected document ID.
export const notifySearch = (event: LifecycleEvent) => {
  const docId = event.result?.docId;
  const syncUrl = process.env.SEARCH_SYNC_URL?.trim();
  const syncToken = process.env.SEARCH_SYNC_TOKEN?.trim();

  if (typeof docId !== 'string' || docId.length === 0 || !syncUrl || !syncToken) {
    return;
  }

  void fetch(syncUrl, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${syncToken}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ docId }),
  })
    .then((response) => {
      if (!response.ok) {
        console.error(`[search] sync notification for ${docId} failed with ${response.status}`);
      }
    })
    .catch((error: unknown) => {
      console.error(`[search] sync notification for ${docId} failed`, error);
    });
};
