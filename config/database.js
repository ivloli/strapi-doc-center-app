module.exports = ({ env }) => ({
  connection: {
    client: env('DATABASE_CLIENT', 'postgres'),
    connection: env('DATABASE_URL')
      ? {
          connectionString: env('DATABASE_URL'),
          ssl: env.bool('DATABASE_SSL', false),
        }
      : {
          host: env('DATABASE_HOST', '127.0.0.1'),
          port: env.int('DATABASE_PORT', 5432),
          database: env('DATABASE_NAME', 'strapi_docs'),
          user: env('DATABASE_USERNAME', 'postgres'),
          password: env('DATABASE_PASSWORD', 'postgres'),
          ssl: env.bool('DATABASE_SSL', false),
        },
  },
});
