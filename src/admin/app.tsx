import type { StrapiApp } from '@strapi/strapi/admin';
import zhHansTranslations from './extensions/translations/zh-Hans.json';

export default {
  config: {
    locales: ['zh-Hans'],
    translations: {
      'zh-Hans': zhHansTranslations,
    },
  },
  bootstrap(_app: StrapiApp) {},
};
