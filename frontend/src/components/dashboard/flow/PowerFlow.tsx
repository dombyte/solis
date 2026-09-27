import React from 'react';
import { useMobile } from '../../../hooks/useMobile';
import { useMediaQuery } from '../../../hooks/useMediaQuery';
import { useSubscription } from '../../../lib/hooks/useSubscription';
import { useRegisterStore } from '../../../lib/stores/useRegisterStore';
import { buildFlowViewModel, POWER_FLOW_KEYS } from './model';
import { FlowDesktop } from './FlowDesktop';
import { FlowMobile } from './FlowMobile';

/**
 * Main PowerFlow component
 * Chooses between desktop and mobile variants based on screen size
 * Subscribes to power flow keys and status keys
 * 
 * Based on prototype App component flow diagram section
 */
export function PowerFlow(): React.ReactElement {
  const isMobile = useMobile();
  const isTablet = useMediaQuery('(max-width: 767px)');
  
  // Subscribe to all power flow keys
  useSubscription([...POWER_FLOW_KEYS]);
  
  // Get data from store
  const registerValues = useRegisterStore(state => state.registerValues);
  const registerMetadata = useRegisterStore(state => state.registerMetadata);
  const isLoading = useRegisterStore(state => state.isLoading);
  const lastUpdated = useRegisterStore(state => state.lastUpdated);
  
  // Determine if data is stale (no updates or still loading)
  const stale = isLoading || lastUpdated === null;
  
  // Build the view model
  const viewModel = buildFlowViewModel(registerValues, registerMetadata, stale);
  
  // Variant selection per spec §14.2:
  // "same determination… tablet shows desktop"
  // mobile = coarse pointer AND width < 768px
  const mobileVariant = isMobile && isTablet;
  
  if (mobileVariant) {
    return <FlowMobile viewModel={viewModel} />;
  }
  
  return <FlowDesktop viewModel={viewModel} />;
}
