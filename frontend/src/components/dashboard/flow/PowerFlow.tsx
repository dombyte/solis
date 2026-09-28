import React, { useMemo } from 'react';
import { useShallow } from 'zustand/react/shallow';
import type { RegisterValue } from '../../../types';
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
 */
export function PowerFlow(): React.ReactElement {
  const isMobile = useMobile();
  const isTablet = useMediaQuery('(max-width: 767px)');
  
  // Subscribe to all power flow keys
  useSubscription([...POWER_FLOW_KEYS]);
  
  // Get data from store
  const registerMetadataByKey = useRegisterStore(state => state.registerMetadataByKey);
  // Only the diagram's keys, compared shallowly: frames that change other keys do not
  // rebuild the flow model (review FE-M2).
  const flowValues = useRegisterStore(useShallow(state => POWER_FLOW_KEYS.map(key => {
    const reg = state.registerMetadataByKey.get(key);
    return reg ? state.registerValues.get(reg.id) : undefined;
  })));
  const registerValues = useMemo(() => {
    const values = new Map<string, RegisterValue>();
    POWER_FLOW_KEYS.forEach((key, i) => {
      const reg = registerMetadataByKey.get(key);
      const value = flowValues[i];
      if (reg && value) values.set(reg.id, value);
    });
    return values;
  }, [flowValues, registerMetadataByKey]);
  const isLoading = useRegisterStore(state => state.isLoading);
  const lastUpdated = useRegisterStore(state => state.lastUpdated);
  const isConnected = useRegisterStore(state => state.isConnected);

  // Stale while loading, disconnected (values are cleared) or before the first frame.
  const stale = isLoading || !isConnected || lastUpdated === null;

  // Build the view model
  const viewModel = buildFlowViewModel(registerValues, registerMetadataByKey, stale);
  
  // Variant selection:
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
