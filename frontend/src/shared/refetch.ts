import { useEffect, useRef } from 'react';

export function useWindowRefetch(refetch: () => void, enabled = true) {
  const refetchRef = useRef(refetch);

  useEffect(() => {
    refetchRef.current = refetch;
  }, [refetch]);

  useEffect(() => {
    if (!enabled) {
      return undefined;
    }

    function triggerRefetch() {
      refetchRef.current();
    }

    function handleVisibilityChange() {
      if (document.visibilityState === 'visible') {
        triggerRefetch();
      }
    }

    window.addEventListener('focus', triggerRefetch);
    window.addEventListener('online', triggerRefetch);
    document.addEventListener('visibilitychange', handleVisibilityChange);

    return () => {
      window.removeEventListener('focus', triggerRefetch);
      window.removeEventListener('online', triggerRefetch);
      document.removeEventListener('visibilitychange', handleVisibilityChange);
    };
  }, [enabled]);
}
