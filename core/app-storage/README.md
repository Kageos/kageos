# App Storage

App Storage is the MinIO-backed file service for kageos. It provides presigned upload credentials, browser/server download URLs, file metadata records, listing, statistics, and delete operations.

System administrators can manage the auditable file inventory from the first-level **System Settings → File Assets** section. The inventory is backed by `file_uploads` and `file_downloads`, supports workspace and service-directory filtering, image thumbnails plus in-page image/video/PDF previews, separate preview/download audit records, and deletion tombstones after the physical object is removed.

This inventory intentionally covers files written through app-storage. Objects written directly to MinIO have no trustworthy service-path, uploader, or business meaning and are not presented as normal platform files. Detect those objects with a low-frequency reconciliation job rather than scanning the entire bucket on every page request. The file-asset UI and API do not expose the MinIO operations console, even when a console URL is configured. Use the [private console access runbook](../../docs/minio-console-access.md) for production maintenance.

## Scope

Current official scope:

- MinIO only.
- Presigned URL upload and download.
- Optional MySQL metadata records through `file_uploads` and `file_downloads`.
- Stable file references in `bucket/object_key` format.
- Browser URLs and server URLs for app containers.

Not in the current MVP:

- Object-level deduplication / instant upload.
- Additional storage backends such as SeaweedFS, COS, OSS, or S3 providers.
- Storage billing, quota enforcement, or cross-tenant governance.

## Layout

```text
core/app-storage/
├── api/v1/          # HTTP handlers
├── service/         # Storage service logic
├── storage/         # MinIO storage adapter and storage interface
├── repository/      # Optional metadata persistence
├── model/           # file_uploads and file_downloads models
├── server/          # Gin server and routes
├── cmd/app/         # Service entrypoint
└── docs/            # API and deployment notes
```

## Upload Flow

1. Client requests `POST /storage/api/v1/upload_token`.
2. App Storage creates a MinIO `presigned_url` credential.
3. Browser uploads the file directly with `PUT upload_url`.
4. Client reports completion through `POST /storage/api/v1/upload_complete` or `batch_upload_complete`.
5. App Storage records metadata when the optional database is configured.

`hash` is accepted as optional file metadata. The SDK uses it for runtime file download caching; App Storage does not currently use it for object deduplication.

## Local Run

Start MinIO through the dev infrastructure:

```bash
bash deploy/dev/scripts/infra.sh podman up -d minio
```

Run the service:

```bash
go build -o bin/app-storage ./core/app-storage/cmd/app
./bin/app-storage
```

Swagger is served by the service at `/swagger/index.html` when enabled.

## Configuration

Current official storage type is `minio`.

```yaml
server:
  port: 8083

storage:
  type: minio
  minio:
    endpoint: localhost:9000
    access_key: minioadmin
    secret_key: minioadmin123
    use_ssl: false
    region: us-east-1
    default_bucket: kageos
  upload:
    max_size: 104857600
    token_expire: 3600
```

Database configuration is optional. Without it, file transfer still works, but upload/download metadata records are skipped.

## Validation

```bash
go test ./core/app-storage/...
```

## Related Docs

- [API examples](docs/API_EXAMPLES.md)
- [Upload flow](docs/UPLOAD_FLOW.md)
- [Upload host handling](docs/UPLOAD_HOST.md)
- [CDN domain notes](docs/CDN_DOMAIN.md)
