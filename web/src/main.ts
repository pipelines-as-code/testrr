import htmx from "htmx.org";
import * as echarts from "echarts";

type ChartSummary = {
  labels: string[];
  pass_rates: number[];
  failures: number[];
  durations: number[];
};

type TestDurationChart = {
  labels: string[];
  durations: number[];
  statuses: string[];
};

declare global {
  interface Window {
    htmx: typeof htmx;
  }
}

window.htmx = htmx;

async function renderCharts(root: ParentNode = document): Promise<void> {
  const nodes = Array.from(root.querySelectorAll<HTMLElement>("[data-chart-endpoint]"));
  await Promise.all(
    nodes.map(async (node) => {
      const endpoint = node.dataset.chartEndpoint;
      const kind = node.dataset.chartKind;
      if (!endpoint || !kind) {
        return;
      }

      const response = await fetch(endpoint, { headers: { Accept: "application/json" } });
      if (!response.ok) {
        node.innerHTML = `<p class="muted">Unable to load chart data.</p>`;
        return;
      }

      const payload = (await response.json()) as ChartSummary | TestDurationChart;
      const chart = echarts.init(node);
      if (kind === "pass-rate") {
        const summary = payload as ChartSummary;
        chart.setOption({
          animationDuration: 450,
          tooltip: { trigger: "axis" },
          xAxis: { type: "category", data: summary.labels },
          yAxis: { type: "value", min: 0, max: 100, axisLabel: { formatter: "{value}%" } },
          series: [
            {
              type: "line",
              data: summary.pass_rates,
              smooth: true,
              lineStyle: { width: 4, color: "#1d7f5f" },
              areaStyle: { color: "rgba(29,127,95,0.16)" },
            },
          ],
        });
        return;
      }

      if (kind === "test-duration") {
        const summary = payload as TestDurationChart;
        chart.setOption({
          animationDuration: 450,
          tooltip: { trigger: "axis" },
          xAxis: { type: "category", data: summary.labels },
          yAxis: { type: "value", axisLabel: { formatter: "{value} ms" } },
          series: [
            {
              type: "line",
              data: summary.durations,
              smooth: true,
              symbolSize: 8,
              lineStyle: { width: 3, color: "#2c6e63" },
              areaStyle: { color: "rgba(44,110,99,0.16)" },
              itemStyle: {
                color: (params: { dataIndex: number }) => {
                  const status = summary.statuses[params.dataIndex];
                  if (status === "failed") {
                    return "#d3485d";
                  }
                  if (status === "skipped") {
                    return "#c68b1f";
                  }
                  return "#2c6e63";
                },
              },
            },
          ],
        });
        return;
      }

      const summary = payload as ChartSummary;
      chart.setOption({
        animationDuration: 450,
        tooltip: { trigger: "axis" },
        xAxis: { type: "category", data: summary.labels },
        yAxis: { type: "value", axisLabel: { formatter: "{value} ms" } },
        series: [
          {
            type: "bar",
            data: summary.durations,
            itemStyle: { color: "#c67b2f", borderRadius: [8, 8, 0, 0] },
          },
        ],
      });
    }),
  );
}

document.addEventListener("DOMContentLoaded", () => {
  void renderCharts();
});

document.body.addEventListener("htmx:afterSwap", (event) => {
  const target = event.target;
  if (target instanceof HTMLElement) {
    void renderCharts(target);
  }
});
