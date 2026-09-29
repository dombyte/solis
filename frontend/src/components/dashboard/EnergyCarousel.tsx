import React, { useRef, useState } from 'react';
import { EnergyCard } from './EnergyCards';
import { SkeletonCard } from '../ui/skeleton';
import type { GroupConfig } from '../../types';

interface EnergyCarouselProps {
  groups: GroupConfig[];
  isLoading: boolean;
  className?: string;
}

// Phone layout for the energy cards: one full-width card at a time (Today first),
// the others side by side to the right, reached by swiping. Native scroll-snap does
// the swiping; the dots only mirror and jump to the current slide.
export function EnergyCarousel({ groups, isLoading, className = '' }: EnergyCarouselProps): React.ReactElement {
  const trackRef = useRef<HTMLDivElement>(null);
  const [active, setActive] = useState(0);

  // Distance between two slides (slide width + gap), read from the DOM so the gap
  // class is the only place that defines it.
  const slideStep = (track: HTMLDivElement): number => {
    const [first, second] = Array.from(track.children) as HTMLElement[];
    return second ? second.offsetLeft - first.offsetLeft : track.clientWidth;
  };

  const handleScroll = () => {
    const track = trackRef.current;
    if (!track) return;
    const step = slideStep(track);
    if (step > 0) setActive(Math.round(track.scrollLeft / step));
  };

  const goTo = (index: number) => {
    const track = trackRef.current;
    if (!track) return;
    track.scrollTo({ left: index * slideStep(track), behavior: 'smooth' });
  };

  return (
    <div className={`min-w-0 ${className}`}>
      <div
        ref={trackRef}
        onScroll={handleScroll}
        className="mobile-scroll flex gap-3 overflow-x-auto snap-x snap-mandatory"
        aria-roledescription="carousel"
        aria-label="Energy summary"
      >
        {groups.map((group, index) => (
          <div
            key={group.id}
            className="w-full flex-shrink-0 snap-center snap-always flex"
            aria-roledescription="slide"
            aria-label={`${index + 1} of ${groups.length}: ${group.title}`}
          >
            {isLoading ? (
              <SkeletonCard className="w-full" />
            ) : (
              <EnergyCard group={group} />
            )}
          </div>
        ))}
      </div>
      <div className="mt-2 flex justify-center gap-1">
        {groups.map((group, index) => (
          <button
            key={group.id}
            type="button"
            onClick={() => goTo(index)}
            aria-label={`Show ${group.title}`}
            aria-current={index === active}
            className="flex h-6 w-6 items-center justify-center"
          >
            <span
              className={`block h-2 rounded-full transition-all-smooth ${
                index === active ? 'w-4 bg-foreground' : 'w-2 bg-muted-foreground/40'
              }`}
            />
          </button>
        ))}
      </div>
    </div>
  );
}
