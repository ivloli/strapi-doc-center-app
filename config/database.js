module.exports = ({ env }) => {
  const ssl = env.bool('DATABASE_SSL', false)
    ? {
        rejectUnauthorized: env.bool('DATABASE_SSL_SELF', false),
      }
    : false;
  const databaseUrl = env('DATABASE_URL');

  return {
    connection: {
      client: env('DATABASE_CLIENT', 'postgres'),
      connection: databaseUrl
        ? {
            connectionString: databaseUrl,
            ssl,
          }
        : {
            host: env('DATABASE_HOST', '127.0.0.1'),
            port: env.int('DATABASE_PORT', 5432),
            database: env('DATABASE_NAME', 'strapi_docs'),
            user: env('DATABASE_USERNAME', 'strapi'),
            password: env('DATABASE_PASSWORD', 'strapi123'),
            ssl,
          },
      pool: {
        min: env.int('DATABASE_POOL_MIN', 2),
        max: env.int('DATABASE_POOL_MAX', 10),
      },
      acquireConnectionTimeout: env.int('DATABASE_CONNECTION_TIMEOUT', 60000),
    },
  };
};
