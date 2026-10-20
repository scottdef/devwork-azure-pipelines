echarts.min.js is not vendored here (1 MB, Apache-2.0).

The /chart page loads /static/echarts/echarts.min.js (go-echarts AssetsHost
is set to /static/echarts/). Without it the page shows a notice and the
same p50/p95 data as an HTML table.

Install it, hash-verified, with:

    scripts/fetch-echarts.sh                 # into this directory; rebuild to embed
    scripts/fetch-echarts.sh /srv/tf/echarts # then: tmplforge serve -assets /srv/tf
