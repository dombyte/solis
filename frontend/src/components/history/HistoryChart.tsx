import React, { useEffect, useLayoutEffect, useRef } from 'react';
import { Chart, registerables } from 'chart.js';
import { useTheme } from '../theme-provider';
import { useMobile } from '../../hooks/useMobile';
import type { ChartData } from '../../types';

// Register all Chart.js components
Chart.register(...registerables);

// Room above the plot area for the top y-axis label, which is centred on its grid line.
const PLOT_TOP_PADDING = 8;

// One pinned y-axis label: its text and its offset (px) from the top of the canvas.
interface YTick {
  label: string;
  y: number;
}

const sameTicks = (a: YTick[], b: YTick[]): boolean =>
  a.length === b.length && a.every((t, i) => t.label === b[i].label && t.y === b[i].y);

interface HistoryChartProps {
  data: ChartData | null;
  className?: string;
  datasetCount?: number;
}

export function HistoryChart({ data, className = '', datasetCount = 0 }: HistoryChartProps): React.ReactElement {
  const chartRef = useRef<HTMLCanvasElement>(null);
  const chartInstanceRef = useRef<Chart | null>(null);
  const [yTicks, setYTicks] = React.useState<YTick[]>([]);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const [datasetVisibility, setDatasetVisibility] = React.useState<Record<string, boolean>>({});

  // Reset dataset visibility when a new `data` object arrives, without a dedicated
  // effect: React's recommended pattern for state that must be re-derived from a
  // changed prop is to adjust it during render, not inside a useEffect.
  const [prevData, setPrevData] = React.useState<ChartData | null>(null);
  if (data !== prevData) {
    setPrevData(data);
    const initialVisibility: Record<string, boolean> = {};
    data?.datasets.forEach((_ds, index) => {
      initialVisibility[String(index)] = true;
    });
    setDatasetVisibility(initialVisibility);
  }

  // Update chart dataset visibility when state changes
  useEffect(() => {
    if (!chartInstanceRef.current || !data) return;

    const chart = chartInstanceRef.current;
    data.datasets.forEach((_ds, index) => {
      const key = String(index);
      const isVisible = datasetVisibility[key] !== false;
      chart.setDatasetVisibility(index, isVisible);
    });
    chart.update();
  }, [datasetVisibility, data]);

  // Open the chart scrolled to the most recent data (labels are sorted oldest -> newest,
  // so "most recent" is the right edge) instead of defaulting to the oldest entries.
  // useLayoutEffect avoids a visible left-to-right jump after the width-driving inner
  // div has committed its minWidth style.
  // The view stays pinned to the right edge until the user scrolls away: the pinned
  // y-axis column only gets its width once Chart.js has published the ticks, which
  // shrinks the scroll container after this first scroll and would leave it short of
  // the end.
  const pinnedToEndRef = useRef(true);
  useLayoutEffect(() => {
    const container = scrollContainerRef.current;
    if (!container) return;
    pinnedToEndRef.current = true;
    container.scrollLeft = container.scrollWidth;
  }, [data]);

  useEffect(() => {
    const container = scrollContainerRef.current;
    if (!container) return;
    const observer = new ResizeObserver(() => {
      if (pinnedToEndRef.current) container.scrollLeft = container.scrollWidth;
    });
    observer.observe(container);
    return () => observer.disconnect();
  }, [data]);

  // Unpin only when the view moved left: the chart's own scroll to the end moves right,
  // and a container that shrinks under it (y-axis column widening) leaves scrollLeft
  // unchanged, so neither must release the pin before the ResizeObserver re-scrolls.
  const lastScrollLeftRef = useRef(0);
  const handleScroll = () => {
    const container = scrollContainerRef.current;
    if (!container) return;
    const atEnd = container.scrollLeft + container.clientWidth >= container.scrollWidth - 1;
    if (atEnd) pinnedToEndRef.current = true;
    else if (container.scrollLeft < lastScrollLeftRef.current) pinnedToEndRef.current = false;
    lastScrollLeftRef.current = container.scrollLeft;
    // Set on the DOM directly: toggling the edge fade needs no re-render per scroll.
    container.dataset.moreLeft = String(container.scrollLeft > 0);
  };

  const toggleDatasetVisibility = (key: string) => {
    setDatasetVisibility(prev => {
      const newVisibility = { ...prev };
      newVisibility[key] = !(prev[key] ?? true);
      return newVisibility;
    });
  };
  const { theme } = useTheme();
  
  // Use pointer-based mobile detection
  const isMobile = useMobile();
  
  // Also track the actual class on the HTML element for system theme changes
  const [currentThemeClass, setCurrentThemeClass] = React.useState(() => {
    const html = document.documentElement;
    return html.classList.contains('dark') ? 'dark' : 'light';
  });

  useEffect(() => {
    const html = document.documentElement;
    const observer = new MutationObserver(() => {
      const newTheme = html.classList.contains('dark') ? 'dark' : 'light';
      if (newTheme !== currentThemeClass) {
        setCurrentThemeClass(newTheme);
      }
    });
    
    observer.observe(html, { attributes: true, attributeFilter: ['class'] });
    return () => observer.disconnect();
  }, [currentThemeClass]);

  useEffect(() => {
    if (!chartRef.current || !data) return;

    // Clean up previous chart instances
    if (chartInstanceRef.current) {
      chartInstanceRef.current.destroy();
      chartInstanceRef.current = null;
    }

    const ctx = chartRef.current.getContext('2d');
    if (!ctx) return;

    // Get colors from CSS variables for theme-aware styling
    const getCssVar = (varName: string) => {
      const element = document.documentElement;
      return getComputedStyle(element).getPropertyValue(varName).trim();
    };

    const foreground = getCssVar('--foreground');
    const mutedForeground = getCssVar('--muted-foreground');
    const borderColorVar = getCssVar('--border');
    const background = getCssVar('--background');

    // Pre-convert all chart colors to RGB once (for performance)
    const chartColors: string[] = [];
    const chartBgColors: string[] = [];
    for (let i = 1; i <= 9; i++) {
      const borderColor = getCssVar(`--chart-${i}`);
      const bgColor = getCssVar(`--chart-bg-${i}`);
      
      const convertToRgb = (colorString: string): string => {
        const canvas = document.createElement('canvas');
        const ctx = canvas.getContext('2d');
        if (!ctx) return colorString;
        try {
          ctx.fillStyle = colorString;
          ctx.fillRect(0, 0, 1, 1);
          return ctx.fillStyle;
        } catch {
          return colorString;
        }
      };
      
      chartColors.push(convertToRgb(borderColor));
      chartBgColors.push(convertToRgb(bgColor));
    }

    // Function to get color for a dataset by index - ensures sequential distinct colors
    const getColorForKey = (_key: string, datasetIndex: number): { border: string; background: string } => {
      const index = datasetIndex % chartColors.length;
      return {
        border: chartColors[index],
        background: chartBgColors[index]
      };
    };

    chartInstanceRef.current = new Chart(ctx, {
      type: 'bar',
      data: {
        labels: data.labels,
        datasets: data.datasets.map((ds, datasetIndex) => {
          const { border, background } = getColorForKey(ds.key, datasetIndex);
          // Scale bar width smoothly with the number of datasets (rather than a binary
          // <=2 cutoff) so bars slim down gradually as more series are overlaid.
          const barPercentage = Math.min(0.85, Math.max(0.4, 0.85 - (datasetCount - 1) * 0.05));
          const categoryPercentage = isMobile ? 0.8 : 0.9;
          return {
            label: ds.label,
            data: ds.data,
            borderColor: border,
            backgroundColor: background,
            borderWidth: 1,
            borderRadius: 4,
            unit: ds.unit,
            // Bar width settings
            barPercentage,
            categoryPercentage,
            // Caps a bar's absolute pixel width so a category with few bars (e.g. a
            // single data point) doesn't stretch to fill the whole plot area -
            // barPercentage/categoryPercentage only control a bar's *fraction* of its
            // category slot, which is the entire plot width when there's 1 category.
            maxBarThickness: isMobile ? 40 : 56,
            // No minBarLength: it also applies to 0, drawing a day with 0 kWh as a
            // visible bar (review FE-M7).
          };
        }),
      },
      // Publish the y ticks after every update (new data, resize, a series toggled) so
      // the pinned HTML axis always matches the grid lines.
      plugins: [{
        id: 'pinnedYAxis',
        afterUpdate: (chart) => {
          const scale = chart.scales.y;
          const next = scale.ticks.map(t => ({
            label: Array.isArray(t.label) ? t.label.join(' ') : String(t.label ?? t.value),
            y: scale.getPixelForValue(t.value),
          }));
          setYTicks(prev => (sameTicks(prev, next) ? prev : next));
        },
      }],
      options: {
        responsive: true,
        maintainAspectRatio: false,
        layout: {
          padding: { top: PLOT_TOP_PADDING },
        },
        plugins: {
          legend: {
            display: false,
          },
          tooltip: {
            backgroundColor: background,
            titleColor: foreground,
            bodyColor: foreground,
            borderColor: borderColorVar,
            borderWidth: 1,
            padding: 10,
            displayColors: true,
            callbacks: {
              // eslint-disable-next-line @typescript-eslint/no-explicit-any
              label: (context: any) => {
                const dataset = context.dataset;
                const unit = dataset.unit || '';
                return `${dataset.label}: ${context.parsed.y ?? '-'}${unit ? ' ' + unit : ''}`;
              }
            }
          },
        },
        scales: {
          x: {
            grid: { 
              display: true,
              color: borderColorVar,
              // Add subtle tick marks
              tickLength: isMobile ? 4 : 2
            },
            ticks: { 
              font: { 
                size: 11,
                family: 'Inter Variable, sans-serif'
              },
              color: mutedForeground,
              // On mobile, allow auto-skip to prevent overlap but prefer to show all
              autoSkip: true,
              // Better padding and rotation for mobile
              autoSkipPadding: isMobile ? 15 : 0,
              // On mobile, use 60 degree rotation to fit more labels
              maxRotation: isMobile ? 60 : 45,
              minRotation: isMobile ? 60 : 45,
              // Add padding between labels
              padding: isMobile ? 8 : 5,
              // Custom callback to format dates in DD.Mon format on mobile
              callback: (_tickValue: string | number, index: number) => {
                const labels = data.labels || [];
                if (!labels[index]) return '';
                
                const label = labels[index];
                
                if (!isMobile) return label;
                
                // On mobile, shorten date labels
                // Handle formats from toLocaleDateString()
                
                // German format: "7. Jan 2024" or "07.01.2024" or "7.1.2024"
                // Extract day and month name
                
                // Format: "DD.MM.YYYY" or "D.M.YYYY"
                const dotDateMatch = label.match(/^(\d{1,2})\.(\d{1,2})\.\d{4}$/);
                if (dotDateMatch) {
                  const day = dotDateMatch[1].padStart(2, '0');
                  const monthNum = parseInt(dotDateMatch[2], 10);
                  const monthNames = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
                  return `${day}.${monthNames[monthNum - 1]}`;
                }
                
                // Format: "D. Mon YYYY" (German locale)
                const germanMonthMatch = label.match(/^(\d{1,2})\.\s*([A-Za-z]{3})\s*\d{4}$/);
                if (germanMonthMatch) {
                  const day = germanMonthMatch[1].padStart(2, '0');
                  const month = germanMonthMatch[2];
                  return `${day}.${month}`;
                }
                
                // Format: "Mon D, YYYY" (English locale)
                const enMonthMatch = label.match(/^([A-Za-z]{3})\s*(\d{1,2})[,\s]*\d{4}$/);
                if (enMonthMatch) {
                  const day = enMonthMatch[2].padStart(2, '0');
                  const month = enMonthMatch[1];
                  return `${day}.${month}`;
                }
                
                // For monthly format (e.g., "Jan 2024"), just use the month
                if (label.match(/^[A-Za-z]{3}\s\d{4}$/)) {
                  return label.split(' ')[0];
                }
                
                // For yearly or other formats, return as-is
                return label;
              }
            },
            border: {
              display: false
            }
          },
          y: {
            grid: {
              color: borderColorVar
            },
            // Labels are rendered as HTML outside the scroll container (see the
            // pinnedYAxis plugin), so they stay pinned to the left while the bars and
            // x-axis scroll. The grid lines stay here with the scrollable plot area.
            ticks: {
              display: false,
            },
            border: {
              display: false
            }
          }
        },
      },
    });

    return () => {
      if (chartInstanceRef.current) {
        chartInstanceRef.current.destroy();
        chartInstanceRef.current = null;
      }
    };
  }, [data, theme, currentThemeClass, datasetCount, isMobile]);

  if (!data) {
    return (
      <div className={`flex items-center justify-center h-64 ${className}`}>
        <p className="text-muted-foreground">No data available</p>
      </div>
    );
  }

  // Calculate the chart's content width based on number of data points and datasets.
  // Scales smoothly with datasetCount (more series per category need more horizontal
  // room) instead of a binary cutoff, and has no artificial floor: `w-full` fills the
  // container when the content is narrower than it, so scrolling only appears once
  // there's genuinely more content than fits (e.g. it no longer forces a sliver of
  // scroll for a single bar).
  const dataPointCount = data.labels?.length || 0;
  const pointWidth = Math.min(160, Math.max(60, 40 + datasetCount * 14));
  const minWidth = Math.min(dataPointCount * pointWidth, 4000);

  // Function to get color for a dataset by index
  const getColorForDataset = (datasetIndex: number): string => {
    const index = datasetIndex % 9; // We have 9 chart colors defined
    return getComputedStyle(document.documentElement).getPropertyValue(`--chart-${index + 1}`).trim();
  };

  // Calculate statistics (min, max, average) per dataset
  const datasetStats = data.datasets.map((ds, index) => {
    const values = ds.data.filter((val): val is number => val !== null && val !== undefined);
    return {
      key: String(index),
      label: ds.label,
      unit: ds.unit || '',
      datasetIndex: index,
      min: values.length > 0 ? Math.min(...values) : null,
      max: values.length > 0 ? Math.max(...values) : null,
      avg: values.length > 0 ? values.reduce((a, b) => a + b, 0) / values.length : null
    };
  });

  // Filter out datasets with no valid values
  const validStats = datasetStats.filter(s => s.min !== null && s.max !== null && s.avg !== null);

  return (
    <div className={`relative w-full ${className}`}>
      <div className="flex w-full" style={{ minHeight: '200px', maxHeight: '500px' }}>
        {/* Pinned y-axis - lives outside the scroll container so it never scrolls. Plain
            text positioned from the chart's own scale: it is never clipped, and the
            invisible copy of the labels sizes the column to the widest one. */}
        <div
          className="relative flex-shrink-0 pr-1 text-[11px] leading-none text-muted-foreground tabular-nums text-right"
          style={{ height: '400px' }}
          aria-hidden="true"
        >
          <div className="invisible h-0 overflow-hidden">
            {yTicks.map(t => <div key={t.label}>{t.label}</div>)}
          </div>
          {yTicks.map(t => (
            <span
              key={t.label}
              className="absolute right-1 -translate-y-1/2 whitespace-nowrap"
              style={{ top: `${t.y}px` }}
            >
              {t.label}
            </span>
          ))}
        </div>
        <div
          ref={scrollContainerRef}
          onScroll={handleScroll}
          className="overflow-x-auto history-chart-scroll flex-1 min-w-0"
        >
          <div className="w-full" style={{ minWidth: `${minWidth}px`, height: '400px' }}>
            <canvas
              ref={chartRef}
              role="img"
              aria-label={`Bar chart of ${data.datasets.length} dataset${data.datasets.length === 1 ? '' : 's'} across ${dataPointCount} period${dataPointCount === 1 ? '' : 's'}`}
              aria-describedby={validStats.length > 0 ? 'history-chart-stats' : undefined}
            />
          </div>
        </div>
      </div>
      {/* Statistics display per category in table format with toggle */}
      {validStats.length > 0 && (
        <div id="history-chart-stats" className="mt-3 sm:px-2 w-full overflow-x-auto">
          <table className="w-full text-xs sm:text-sm border-collapse">
            <thead>
              <tr className="border-b border-border">
                <th className="text-left py-2 pl-0 pr-1.5 sm:px-3 font-medium text-foreground">Register</th>
                <th className="text-right py-2 px-1 sm:px-3 font-medium text-foreground">Min</th>
                <th className="text-right py-2 px-1 sm:px-3 font-medium text-foreground">Max</th>
                <th className="text-right py-2 px-1 sm:px-3 font-medium text-foreground">Average</th>
              </tr>
            </thead>
            <tbody>
              {validStats.map((stat, index) => {
                const isVisible = datasetVisibility[stat.key] !== false;
                const dotColor = getColorForDataset(stat.datasetIndex);
                return (
                  <tr key={`${stat.key}-${index}`} className="border-b border-border/50">
                    <td className="text-left py-2 pl-0 pr-1.5 sm:px-3">
                      <button
                        onClick={() => toggleDatasetVisibility(stat.key)}
                        className={`flex items-center gap-2 text-left w-full text-foreground hover:text-primary transition-colors ${
                          !isVisible ? 'line-through opacity-50' : ''
                        }`}
                        style={{ background: 'none', border: 'none', cursor: 'pointer' }}
                      >
                        <span 
                          className="w-3 h-3 rounded-full flex-shrink-0" 
                          style={{ backgroundColor: dotColor }}
                        />
                        {stat.label}
                      </button>
                    </td>
                    {[stat.min, stat.max, stat.avg].map((v, i) => (
                      // The number never breaks. On phones the unit always sits on its own
                      // line below it (the same in every cell, instead of wrapping only
                      // where a value is long). With the tighter phone padding this keeps
                      // all three columns visible at 360px, even for 4-digit yearly values
                      // next to "Energy Consumption".
                      <td key={i} className="text-right py-2 px-1 sm:px-3 text-muted-foreground whitespace-nowrap tabular-nums">
                        {v?.toFixed(2)}{stat.unit && <span className="block text-[10px] sm:inline sm:text-xs"><span className="hidden sm:inline"> </span>{stat.unit}</span>}
                      </td>
                    ))}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
