import React from 'react';
import { cn } from '../../lib/utils';

interface SkeletonProps {
  className?: string;
}

export function Skeleton({ className }: SkeletonProps): React.ReactElement {
  return (
    <div
      className={cn(
        'animate-pulse rounded bg-muted',
        className
      )}
    />
  );
}

// Skeleton for a card
export function SkeletonCard({ className }: SkeletonProps): React.ReactElement {
  return (
    <div className={cn('border rounded-2xl p-4 space-y-3 bg-card/80 backdrop-blur-lg shadow-lg', className)}>
      <Skeleton className="h-6 w-3/4" />
      <Skeleton className="h-4 w-1/2" />
      <Skeleton className="h-20 w-full" />
    </div>
  );
}
