import htmx from "htmx.org";
import * as echarts from "echarts";

type ChartSummary = {
  labels: string[];
  pass_rates: number[];
  failures: number[];
  durations: number[];
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

      const payload = (await response.json()) as ChartSummary;
      const chart = echarts.init(node);
      if (kind === "pass-rate") {
        chart.setOption({
          animationDuration: 450,
          tooltip: { trigger: "axis" },
          xAxis: { type: "category", data: payload.labels },
          yAxis: { type: "value", min: 0, max: 100, axisLabel: { formatter: "{value}%" } },
          series: [
            {
              type: "line",
              data: payload.pass_rates,
              smooth: true,
              lineStyle: { width: 4, color: "#1d7f5f" },
              areaStyle: { color: "rgba(29,127,95,0.16)" },
            },
          ],
        });
        return;
      }

      chart.setOption({
        animationDuration: 450,
        tooltip: { trigger: "axis" },
        xAxis: { type: "category", data: payload.labels },
        yAxis: { type: "value", axisLabel: { formatter: "{value} ms" } },
        series: [
          {
            type: "bar",
            data: payload.durations,
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
