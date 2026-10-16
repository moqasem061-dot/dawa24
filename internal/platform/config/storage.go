package config

// Storage configures an optional S3-compatible object storage (AWS S3, MinIO).
// When not configured, uploads are stored locally on the platform volume under UPLOAD_DIR.
type Storage struct {
	Endpoint        string // MinIO or S3 endpoint
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool // MinIO needs this; AWS S3 does not
	PublicBaseURL   string
}

// Configured reports whether external object storage is enabled and validly configured.
// If neither an endpoint nor access credentials are provided, it returns false,
// signaling that files are stored locally on the platform storage.
func (s Storage) Configured() bool {
	if s.Bucket == "" {
		return false
	}
	return s.Endpoint != "" || (s.AccessKeyID != "" && s.SecretAccessKey != "")
}

func loadStorage(fail func(format string, args ...any), env Env, cliOnly bool) Storage {
	endpoint := getStr("STORAGE_ENDPOINT", "")
	accessKey := getStr("STORAGE_ACCESS_KEY_ID", "")
	secretKey := getStr("STORAGE_SECRET_ACCESS_KEY", "")

	// Only set a default bucket if endpoint or credentials are configured.
	// Otherwise, leave bucket empty to avoid false-positive object storage initialization.
	defaultBucket := ""
	if endpoint != "" || (accessKey != "" && secretKey != "") {
		defaultBucket = "dawa24"
	}
	bucket := getStr("STORAGE_BUCKET", defaultBucket)

	cfg := Storage{
		Endpoint:        endpoint,
		Region:          getStr("STORAGE_REGION", "us-east-1"),
		Bucket:          bucket,
		AccessKeyID:     accessKey,
		SecretAccessKey: secretKey,
		UsePathStyle:    getBool("STORAGE_USE_PATH_STYLE", true),
		PublicBaseURL:   getStr("STORAGE_PUBLIC_BASE_URL", ""),
	}

	if env.IsProd() && !cliOnly {
		if cfg.Endpoint != "" && (cfg.AccessKeyID == "" || cfg.SecretAccessKey == "") {
			fail("STORAGE_ACCESS_KEY_ID and STORAGE_SECRET_ACCESS_KEY are required when STORAGE_ENDPOINT is configured")
		}
	}

	return cfg
}
