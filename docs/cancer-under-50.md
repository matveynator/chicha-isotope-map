# Cancer deaths under 50 overlay (WHO)

## Overview
The `-cancer-under-50` flag enables a country-level overlay that shades each country by the
number of cancer deaths under age 50. The frontend uses the embedded Natural Earth GeoJSON
polygons and colors them green → yellow → red based on the latest available value per country.

## Data source
The backend polls the WHO Global Health Observatory (GHO) API and exposes a cached snapshot at
`/api/cancer_under_50`. The default endpoint is:

```
https://ghoapi.azureedge.net/api/GHECAUSES?$filter=Dim1%20eq%20'SEX_BTSX'%20and%20Dim2Value%20eq%20'0-49%20years'%20and%20Dim3Value%20eq%20'Malignant%20neoplasms'
```

This uses the GHECAUSES dataset with the "Both sexes" dimension and the 0–49 years age group.
If the indicator or filter needs adjustment for a newer WHO schema, set a custom URL with
`-cancer-under-50-source`.

## Flags
- `-cancer-under-50`: enable the overlay and background polling.
- `-cancer-under-50-source`: override the default WHO API URL.

## Notes
- The service refreshes the dataset once every 24 hours.
- The overlay is not enabled by default; it appears as a toggle in the Leaflet layer control.
