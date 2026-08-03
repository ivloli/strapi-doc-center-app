type LifecycleEvent = {
  result?: {
    docId?: unknown;
    value?: unknown;
  };
};

// Search remains outside Strapi: lifecycle hooks signal a document or menu visibility change.
export const notifySearch = (event: LifecycleEvent) => {
  const docId = event.result?.docId;
  const menuValue = event.result?.value;
  const syncUrl = process.env.SEARCH_SYNC_URL?.trim();
  const syncToken = process.env.SEARCH_SYNC_TOKEN?.trim();

  const hasDocID = typeof docId === 'string' && docId.length > 0;
  const hasMenuValue = typeof menuValue === 'string' && menuValue.length > 0;
  if ((!hasDocID && !hasMenuValue) || !syncUrl || !syncToken) {
    return;
  }

  void fetch(syncUrl, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${syncToken}`,
      'Content-Type': 'application/json',
    },
    // 既有菜单不保存 docId，使用 value 触发搜索服务的可见性对账。
    body: JSON.stringify({ docId: hasDocID ? docId : undefined, menuValue: hasMenuValue ? menuValue : undefined }),
  })
    .then((response) => {
      if (!response.ok) {
        console.error(`[search] sync notification for ${docId ?? menuValue} failed with ${response.status}`);
      }
    })
    .catch((error: unknown) => {
      console.error(`[search] sync notification for ${docId ?? menuValue} failed`, error);
    });
};
