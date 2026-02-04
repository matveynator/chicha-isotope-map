# Cancer deaths under 50 overlay (GCO)

## Overview
The `-cancer-under-50` flag enables a country-level overlay that shades each country by the
number of cancer deaths under age 50. The frontend uses the embedded Natural Earth GeoJSON
polygons and colors them green → yellow → red based on the latest available value per country.

## Data source
The backend polls the Global Cancer Observatory (GCO) API and exposes a cached snapshot at
`/api/cancer_under_50`. The default endpoint is:

```
https://gco.iarc.fr/138a2c5a-1b8a-4583-8ec6-91642ad6d28b
```

If the GCO share page returns HTML, the service will look for an embedded API URL and follow it
automatically. If you need to point at a different dataset, set a custom URL with
`-cancer-under-50-source`.

## Flags
- `-cancer-under-50`: enable the overlay and background polling.
- `-cancer-under-50-source`: override the default GCO API URL.

## Notes
- The service refreshes the dataset once every 24 hours.
- The overlay is not enabled by default; it appears as a toggle in the Leaflet layer control.
- Startup logs include the exact GCO source URL for quick verification in a browser.
