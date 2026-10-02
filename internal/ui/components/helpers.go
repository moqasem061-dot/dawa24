package components

// Asset resolves bundled component images through the shared content-hash cache.
// ui installs the resolver; isolated template tests retain a usable static URL.
var Asset = func(path string) string { return path }
