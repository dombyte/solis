import React, { useEffect, useRef } from 'react';
import { Chart, registerables } from 'chart.js';
import { useTheme } from '../theme-provider';
import type { ChartData } from '../../types';

// Register all Chart.js components
Chart.register(...registerables);

interface HistoryChartProps {
  data: ChartData | null;
  className?: string;
  datasetCount?: number;
}

export function HistoryChart({ data, className = '', datasetCount = 0 }: HistoryChartProps): React.ReactElement {
  const chartRef = useRef<HTMLCanvasElement>(null);
  const chartInstanceRef = useRef<Chart | null>(null);
  const [datasetVisibility, setDatasetVisibility] = React.useState<Record<string, boolean>>({});
  
  // Initialize and update dataset visibility when data changes
  useEffect(() => {
    if (!data) return;
    
    // Initialize all datasets as visible
    const initialVisibility: Record<string, boolean> = {};
    data.datasets.forEach((ds, index) => {
      initialVisibility[ds.label || `dataset-${index}`] = true;
    });
    setDatasetVisibility(initialVisibility);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data]);

  // Update chart dataset visibility when state changes
  useEffect(() => {
    if (!chartInstanceRef.current || !data) return;
    
    const chart = chartInstanceRef.current;
    data.datasets.forEach((ds, index) => {
      const label = ds.label || `dataset-${index}`;
      const isVisible = datasetVisibility[label] !== false;
      chart.setDatasetVisibility(index, isVisible);
    });
    chart.update();
  }, [datasetVisibility, data]);

  const toggleDatasetVisibility = (label: string) => {
    setDatasetVisibility(prev => {
      const newVisibility = { ...prev };
      newVisibility[label] = !(prev[label] ?? true);
      return newVisibility;
    });
  };
  const { theme } = useTheme();
  
  // State for mobile detection
  const [isMobile, setIsMobile] = React.useState(false);

  // Check mobile on mount and resize
  useEffect(() => {
    const checkMobile = () => {
      const mobile = typeof window !== 'undefined' ? window.innerWidth <= 768 : false;
      setIsMobile(mobile);
    };

    checkMobile();
    window.addEventListener('resize', checkMobile);
    return () => window.removeEventListener('resize', checkMobile);
  }, []);
  
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
    
    // Clean up previous chart instance
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
          // Calculate bar width based on number of datasets
          // For 1-2 datasets, use smaller bars to fit more
          // On mobile, use slightly smaller bars to create more space between dates
          const barPercentage = isMobile ? (datasetCount <= 2 ? 0.5 : 0.7) : (datasetCount <= 2 ? 0.4 : 0.8);
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
            // Minimum bar length in pixels to prevent bars from getting too thin
            minBarLength: isMobile ? 8 : 10,
          };
        }),
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
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
                size: isMobile ? 10 : 10,
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
            ticks: { 
              font: { 
                size: 10,
                family: 'Inter Variable, sans-serif'
              },
              color: mutedForeground
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

  // Calculate minimum width based on number of data points and datasets
  // When 1-2 categories are selected, reduce spacing to show more time period
  const dataPointCount = data.labels?.length || 0;
  const pointWidth = datasetCount <= 2 ? 60 : 80;
  const minWidth = Math.min(Math.max(dataPointCount * pointWidth, 800), 4000);

  // Function to get color for a dataset by index
  const getColorForDataset = (datasetIndex: number): string => {
    const index = datasetIndex % 9; // We have 9 chart colors defined
    return getComputedStyle(document.documentElement).getPropertyValue(`--chart-${index + 1}`).trim();
  };

  // Calculate statistics (min, max, average) per dataset
  const datasetStats = data.datasets.map((ds, index) => {
    const values = ds.data.filter((val): val is number => val !== null && val !== undefined);
    return {
      key: ds.label || `dataset-${index}`,
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
      <div 
        className="overflow-x-auto history-chart-scroll w-full"
        style={{ minHeight: '200px', maxHeight: '500px' }}
      >
        <div className="w-full" style={{ minWidth: `${minWidth}px`, height: '400px' }}>
          <canvas ref={chartRef} />
        </div>
      </div>
      {/* Statistics display per category in table format with toggle */}
      {validStats.length > 0 && (
        <div className="mt-3 px-2 w-full overflow-x-auto">
          <table className="w-full text-sm border-collapse">
            <thead>
              <tr className="border-b border-border">
                <th className="text-left py-2 px-3 font-medium text-foreground">Register</th>
                <th className="text-right py-2 px-3 font-medium text-foreground">Min</th>
                <th className="text-right py-2 px-3 font-medium text-foreground">Max</th>
                <th className="text-right py-2 px-3 font-medium text-foreground">Average</th>
              </tr>
            </thead>
            <tbody>
              {validStats.map((stat, index) => {
                const isVisible = datasetVisibility[stat.key] !== false;
                const dotColor = getColorForDataset(stat.datasetIndex);
                return (
                  <tr key={`${stat.key}-${index}`} className="border-b border-border/50">
                    <td className="text-left py-2 px-3">
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
                    <td className="text-right py-2 px-3 text-muted-foreground">
                      {stat.min?.toFixed(2)}{stat.unit && ` ${stat.unit}`}
                    </td>
                    <td className="text-right py-2 px-3 text-muted-foreground">
                      {stat.max?.toFixed(2)}{stat.unit && ` ${stat.unit}`}
                    </td>
                    <td className="text-right py-2 px-3 text-muted-foreground">
                      {stat.avg?.toFixed(2)}{stat.unit && ` ${stat.unit}`}
                    </td>
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
