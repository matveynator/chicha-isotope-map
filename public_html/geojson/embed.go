package geojson

import _ "embed"

// Countries stores the Natural Earth country dataset so other packages can
// stay detached from the filesystem at runtime.
//
//go:embed ne_10m_admin_0_countries.geojson
var Countries []byte
