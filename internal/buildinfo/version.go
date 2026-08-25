// Package buildinfo 保存由构建流水线注入的版本信息。
package buildinfo

// Version 是当前构建版本。正式构建通过 -ldflags
// "-X bb_erp_echo/internal/buildinfo.Version=1.2.3" 注入；本地开发保持 dev。
var Version = "dev"
