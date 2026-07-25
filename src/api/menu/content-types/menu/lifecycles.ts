import { notifySearch } from '../../../../search/notify';

export default {
  afterCreate(event: { result?: { docId?: unknown } }) {
    notifySearch(event);
  },

  afterUpdate(event: { result?: { docId?: unknown } }) {
    notifySearch(event);
  },

  afterDelete(event: { result?: { docId?: unknown } }) {
    notifySearch(event);
  },
};
