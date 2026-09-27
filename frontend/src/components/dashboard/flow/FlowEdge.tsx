import React from 'react';

/**
 * Animated flow edge component
 * Uses CSS keyframes instead of requestAnimationFrame for better performance
 * and to respect prefers-reduced-motion
 * 
 * Based on prototype FlowEdge component
 */
interface FlowEdgeProps {
  d: string;          // SVG path definition
  active: boolean;    // Whether the edge is active (non-zero flow)
  reverse: boolean;   // Whether to reverse the animation direction
  color: string;      // CSS color token (e.g., 'var(--color-flow-pv)')
  className?: string;
}

// Animation duration in seconds (matches prototype: ~42px/s)
// 17px dash pattern / 42px/s = ~0.4s for one cycle
const ANIMATION_DURATION = '0.4s';

export function FlowEdge({ 
  d, 
  active, 
  reverse, 
  color,
  className = '' 
}: FlowEdgeProps): React.ReactElement {
  if (!active) {
    // When inactive, just render the track with no animation
    return (
      <g className={className}>
        <path 
          d={d} 
          fill="none" 
          stroke="var(--color-flow-track)" 
          strokeWidth={7} 
          strokeLinecap="round" 
        />
      </g>
    );
  }

  // Active edge with glow and dash animations
  const animationDirection = reverse ? 'reverse' : 'normal';
  
  return (
    <g className={className}>
      {/* Track (background) */}
      <path 
        d={d} 
        fill="none" 
        stroke="var(--color-flow-track)" 
        strokeWidth={7} 
        strokeLinecap="round" 
      />
      
      {/* Glow effect (blur layer) - animated with flowGlow keyframes */}
      <path
        d={d}
        fill="none"
        stroke={color}
        strokeWidth={11}
        strokeLinecap="round"
        strokeDasharray="10 12"
        strokeDashoffset={0}
        opacity={0.25}
        style={{
          filter: 'blur(2px)',
          animation: `flowGlow ${ANIMATION_DURATION} linear infinite`,
          animationDirection,
          transition: 'opacity 0.5s ease',
        }}
      />
      
      {/* Main dash effect - animated with flowDash keyframes */}
      <path
        d={d}
        fill="none"
        stroke={color}
        strokeWidth={5.5}
        strokeLinecap="round"
        strokeDasharray="8 9"
        strokeDashoffset={0}
        opacity={1}
        style={{
          animation: `flowDash ${ANIMATION_DURATION} linear infinite`,
          animationDirection,
          transition: 'opacity 0.5s ease',
        }}
      />
    </g>
  );
}
