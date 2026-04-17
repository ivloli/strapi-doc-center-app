module.exports = ({ env }) => ({
  upload: {
    config: {
      provider: 'aws-s3',
      providerOptions: {
        s3Options: {
          credentials: {
            accessKeyId: env('S3_ACCESS_KEY_ID'),
            secretAccessKey: env('S3_SECRET_ACCESS_KEY'),
          },
          region: env('S3_REGION', 'us-east-1'),
          endpoint: env('S3_ENDPOINT'),
          forcePathStyle: env.bool('S3_FORCE_PATH_STYLE', true),
        },
        params: {
          Bucket: env('S3_BUCKET', 'files'),
        },
      },
    },
  },
});
