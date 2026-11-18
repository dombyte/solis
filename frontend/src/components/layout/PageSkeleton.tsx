import React from 'react';
import { useLocation } from 'react-router-dom';
import { Page } from './Page';
import { pageTitle } from './nav';
import { Skeleton, SkeletonCard } from '../ui/skeleton';

/**
 * Placeholder while a page's code loads: the real page frame and title with skeleton
 * cards, so the layout is already in place instead of a centered spinner.
 */
export function PageSkeleton(): React.ReactElement {
  const { pathname } = useLocation();

  return (
    <Page title={pageTitle(pathname)}>
      <div role="status" aria-label="Loading page" className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <SkeletonCard className="w-full" />
        <SkeletonCard className="w-full" />
        <Skeleton className="h-48 w-full lg:col-span-2 rounded-xl" />
      </div>
    </Page>
  );
}
