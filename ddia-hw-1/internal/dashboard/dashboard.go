package dashboard

import (
	"fmt"
	"strconv"

	"highload/ddia-hw-1/internal/metrics"
)

const UID = "ddia-hw-1"
const DatasourceUID = "victoriametrics"

type Object = map[string]any

func Build(routes []metrics.Route) Object {
	ds := Object{"type": "prometheus", "uid": DatasourceUID}
	variables := []any{
		variable(ds, "deployment", "Deployment", "label_values(http_requests_total, deployment)"),
		variable(ds, "datacenter", "Datacenter", `label_values(http_requests_total{deployment=~"${deployment:regex}"}, datacenter)`),
		variable(ds, "pod", "Pod", `label_values(http_requests_total{deployment=~"${deployment:regex}",datacenter=~"${datacenter:regex}"}, pod)`),
	}
	panels := []any{}
	filters := `deployment=~"${deployment:regex}",datacenter=~"${datacenter:regex}",pod=~"${pod:regex}"`
	for i, route := range routes {
		selector := filters + ",endpoint=" + strconv.Quote(route.ID)
		latency := []any{}
		for j, q := range []float64{.5, .75, .9, .95, .99} {
			expr := fmt.Sprintf(`histogram_quantile(%g, sum by (le) (rate(%s_bucket{%s}[$__rate_interval])))`, q, metrics.Duration, selector)
			latency = append(latency, target(string(rune('A'+j)), expr, fmt.Sprintf("p%g", q*100)))
		}
		panels = append(panels,
			panel(ds, 3*i+1, route.ID+" · Latency", "s", 0, 8*i, latency),
			panel(ds, 3*i+2, route.ID+" · Throughput", "reqps", 8, 8*i, []any{
				target("A", fmt.Sprintf(`sum(rate(%s{%s}[$__rate_interval]))`, metrics.Requests, selector), "requests/s"),
			}),
			panel(ds, 3*i+3, route.ID+" · Errors by HTTP status", "reqps", 16, 8*i, []any{
				target("A", fmt.Sprintf(`sum by (status) (rate(%s{%s,status=~"[45].."}[$__rate_interval]))`, metrics.Requests, selector), "{{status}}"),
			}),
		)
	}
	return Object{
		"id": nil, "uid": UID, "title": "DDIA · HTTP golden signals", "editable": false,
		"schemaVersion": 41, "version": 0, "tags": []string{"ddia", "generated"},
		"timezone": "browser", "refresh": "5s", "time": Object{"from": "now-15m", "to": "now"},
		"graphTooltip": 1, "panels": panels, "templating": Object{"list": variables},
		"annotations": Object{"list": []any{}}, "links": []any{},
	}
}

func target(ref, expression, legend string) Object {
	return Object{"refId": ref, "expr": expression, "legendFormat": legend, "range": true, "editorMode": "code"}
}

func variable(ds Object, name, label, query string) Object {
	return Object{"name": name, "label": label, "type": "query", "datasource": ds,
		"query": Object{"query": query, "refId": name}, "definition": query,
		"refresh": 1, "sort": 1, "multi": true, "includeAll": true, "allValue": ".*",
		"current": Object{"text": "All", "value": "$__all"}, "options": []any{},
	}
}

func panel(ds Object, id int, title, unit string, x, y int, targets []any) Object {
	return Object{"id": id, "type": "timeseries", "title": title, "datasource": ds,
		"gridPos": Object{"x": x, "y": y, "w": 8, "h": 8}, "targets": targets,
		"fieldConfig": Object{"defaults": Object{"unit": unit, "min": 0,
			"color":  Object{"mode": "palette-classic"},
			"custom": Object{"drawStyle": "line", "lineInterpolation": "linear", "lineWidth": 2, "fillOpacity": 12, "spanNulls": false},
		}, "overrides": []any{}},
		"options": Object{"legend": Object{"displayMode": "list", "placement": "bottom", "calcs": []string{"lastNotNull"}}, "tooltip": Object{"mode": "multi", "sort": "desc"}},
	}
}
