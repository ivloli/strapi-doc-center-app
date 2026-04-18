module.exports = ({ env }) => {
  const endpointRaw = env('S3_ENDPOINT', env('MINIO_ENDPOINT', ''));
  const useSsl = env.bool('S3_USE_SSL', env.bool('MINIO_USE_SSL', true));
  const endpoint = endpointRaw
    ? endpointRaw.startsWith('http://') || endpointRaw.startsWith('https://')
      ? endpointRaw
      : `${useSsl ? 'https' : 'http'}://${endpointRaw}`
    : undefined;

  return {
    upload: {
      config: {
        provider: 'aws-s3',
        providerOptions: {
          s3Options: {
            credentials: {
              accessKeyId: env('S3_ACCESS_KEY_ID', env('AWS_ACCESS_KEY_ID')),
              secretAccessKey: env('S3_SECRET_ACCESS_KEY', env('AWS_ACCESS_SECRET')),
            },
            region: env('S3_REGION', env('AWS_REGION', 'us-east-1')),
            endpoint,
            forcePathStyle: env.bool('S3_FORCE_PATH_STYLE', !!endpointRaw),
          },
          params: {
            Bucket: env('S3_BUCKET', env('AWS_BUCKET')),
          },
          rootPath: env('S3_ROOT_PATH', env('AWS_ROOT_PATH', 'docs-center')),
          signedUrlExpires: env.int('S3_SIGNED_URL_EXPIRES', 600),
        },
        actionOptions: {
          upload: {},
          uploadStream: {},
          delete: {},
        },
      },
    },
  };
};
