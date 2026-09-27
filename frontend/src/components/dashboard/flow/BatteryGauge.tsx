import React from 'react';

/**
 * Segmented battery icon with 5 x 20% segments
 * Based on prototype BatterySegments component
 */
interface BatteryGaugeProps {
  soc: number;           // State of charge (0-100)
  className?: string;
}

const GAP = 2;
const SEG_COUNT = 5;

/**
 * Get the color for a segment based on SOC percentage
 * Uses CSS tokens from index.css
 */
function getSegmentColor(segmentPercent: number): string {
  if (segmentPercent <= 20) return 'var(--color-flow-batt-low)';
  if (segmentPercent <= 40) return 'var(--color-flow-batt-mid)';
  return 'var(--color-flow-batt-high)';
}

export function BatteryGauge({ soc, className = '' }: BatteryGaugeProps): React.ReactElement {
  const fillPct = Math.max(0, Math.min(100, soc));
  
  // Calculate dimensions
  const totalWidth = 46;
  const totalHeight = 26;
  const segWidth = (totalWidth - GAP * (SEG_COUNT - 1)) / SEG_COUNT;

  return (
    <g className={className}>
      {/* Battery outline/background */}
      <rect 
        x={2} 
        y={1} 
        width={totalWidth} 
        height={totalHeight} 
        rx={3} 
        fill="var(--color-flow-batt-icon-bg)"
        stroke="var(--color-flow-batt-icon-stroke)"
        strokeWidth={1.5}
      />
      
      {/* Battery terminal indicator (positive side) */}
      <rect 
        x={totalWidth + 2 + 1} 
        y={(totalHeight - 12) / 2 + 1}
        width={4} 
        height={12} 
        rx={1.2} 
        fill="var(--color-flow-batt-icon-stroke)"
      />
      
      {/* 5 segments */}
      {Array.from({ length: SEG_COUNT }, (_, i) => {
        const segPct = (i + 1) * 20; // 20, 40, 60, 80, 100
        const filled = fillPct >= segPct - 10; // Fill if SOC >= segment threshold
        const color = filled ? getSegmentColor(segPct) : 'transparent';
        
        return (
          <rect
            key={i}
            x={2 + 1.5 + GAP / 2 + i * (segWidth + GAP)}
            y={3.5}
            width={segWidth - 1}
            height={totalHeight - 7}
            rx={1.5}
            fill={color}
            className="battery-segment"
            style={{ transition: 'fill 0.5s ease' }}
          />
        );
      })}
    </g>
  );
}
