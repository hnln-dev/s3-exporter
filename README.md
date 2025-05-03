# S3 Exporter

## Config

| Long flag          | Description          | Default |
|--------------------|----------------------|---------|
| `--listen-address` | Address to listen on | `:9340` |
| `--tls.cert-file`  | TLS cert file        |         |
| `--tls.key-file`   | TLS key file         |         |

## Usage

AWS Access Key ID and AWS Secret Access Key can be passed via basic auth.
If no credentials are present anonymous authentication is attempted.

| Parameter        | Description         | Required | Default                             |
|------------------|---------------------|----------|-------------------------------------|
| `endpoint`       | S3 endpoint URL     | no       | `https://s3.<region>.amazonaws.com` |
| `bucket`         | S3 bucket name      | no       |                                     |
| `prefix`         | S3 prefix           | no       |                                     |
| `delimiter`      | S3 delimiter        | no       |                                     |
| `region`         | S3 region           | yes      |                                     |
| `forcePathStyle` | S3 force path style | no       | `false`                             |
| `depth`          | S3 depth            | no       |                                     |

If `bucket` is not specified, the S3 Exporter will list all buckets in the specified region.

```yaml
scrape_configs:
  - job_name: s3-exporter
    static_configs:
      - targets: ['bucket-a', 'bucket-b']
        labels:
          region: eu-central-1
    metrics_path: /probe
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_bucket
      - source_labels: [region]
        target_label: __param_region
      - target_label: __address__
        replacement: localhost:8080  # Address of the S3 Exporter.
```

## `delimiter` and `depth`

The `delimiter` and `depth` parameters are used to control the way the S3 Exporter retrieves hierarchies from S3.
The `delimiter` parameter is used to group objects by a common prefix, while the `depth` parameter is used to limit the number of levels of prefixes to retrieve.

For example, if you have the following S3 bucket structure:

```
bucket-a:
    ├── folder1
    │   ├── file1.txt
    │   └── file2.txt
    ├── folder2
    │   ├── file3.txt
    │   └── file4.txt
    └── folder3
        ├── file5.txt
        └── file6.txt
```

You can use the `delimiter` and `depth` parameters to get the size and count of objects in each folder.

```bash
curl http://127.0.0.1:9340/probe -G -d bucket=bucket-a -d delimiter=/ -d depth=1 | grep -ve '^#'
```

```prometheus
s3_object_count{bucket="bucket-a",prefix="folder1/"} 2
s3_object_count{bucket="bucket-a",prefix="folder2/"} 2
s3_object_count{bucket="bucket-a",prefix="folder3/"} 2
s3_object_size_sum_bytes{bucket="bucket-a",prefix="folder1/"} 1234
s3_object_size_sum_bytes{bucket="bucket-a",prefix="folder2/"} 1234
s3_object_size_sum_bytes{bucket="bucket-a",prefix="folder3/"} 1234
```
