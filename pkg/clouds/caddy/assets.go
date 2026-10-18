// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package caddy

import "embed"

// FS is the asset tree: `assets/Caddyfile` (the snippet library) and
// `assets/pages/*.html` (the error pages it serves). Consumers copy the tree
// to /etc/caddy — as a ConfigMap-backed volume in Kubernetes
// (kubernetes.EmbedFSToTextVolumes) or as files on the host in the VM path.
//
//go:embed assets/*
var FS embed.FS

// AssetsDir is the prefix every path in FS carries. Kept as a constant because
// both consumers have to strip it, and a literal in two packages is how the
// next move of these files breaks one of them silently.
const AssetsDir = "assets"

// CaddyfilePath is the snippet library inside FS.
const CaddyfilePath = AssetsDir + "/Caddyfile"
