import React from 'react';
import { useMobile } from '../../../hooks/useMobile';
import { useMediaQuery } from '../../../hooks/useMediaQuery';
import { useSubscription } from '../../../lib/hooks/useSubscription';
import { useRegisterStore } from '../../../lib/stores/useRegisterStore';
import { Card } from '../../ui/card';
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
  const registerMetadataByKey = useRegisterStore(state => state.registerMetadataByKey);
  const isLoading = useRegisterStore(state => state.isLoading);
  const lastUpdated = useRegisterStore(state => state.lastUpdated);
  const isConnected = useRegisterStore(state => state.isConnected);

  // Stale while loading, disconnected (values are cleared) or before the first frame.
  const stale = isLoading || !isConnected || lastUpdated === null;

  // Build the view model
  const viewModel = buildFlowViewModel(registerValues, registerMetadataByKey, stale);
  
  // Variant selection per spec §14.2:
  // "same determination… tablet shows desktop"
  // mobile = coarse pointer AND width < 768px
  const mobileVariant = isMobile && isTablet;

  return (
    // h-full + centering: the grid row stretches every card to the same height, so
    // this fills that height (instead of staying content-sized and leaving a gap
    // beneath it) and centers the diagram within any extra vertical space.
    <Card className="w-full min-w-0 h-full p-3 sm:p-4 flex flex-col justify-center">
      {mobileVariant ? <FlowMobile viewModel={viewModel} /> : <FlowDesktop viewModel={viewModel} />}
    </Card>
  );
}
