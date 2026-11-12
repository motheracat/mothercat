"use client";

import { useCallback, useEffect, useState } from "react";

import { getCompanyActivity, getMyActivity, getPending } from "@/lib/analytics/api";
import type {
  AnalyticsRange,
  CompanyActivityAnalytics,
  MyActivityAnalytics,
  PendingAnalytics,
} from "@/lib/analytics/types";

type QueryState<T> = { key: string; data: T | null; error: string | null } | null;
type AnalyticsResult<T> = { data: T | null; isLoading: boolean; error: string | null; retry: () => void };

function useAnalyticsQuery<T>(
  key: string,
  load: () => Promise<T>,
): AnalyticsResult<T> {
  const [queryState, setQueryState] = useState<QueryState<T>>(null);
  const [retryKey, setRetryKey] = useState(0);
  const [isRetrying, setIsRetrying] = useState(false);

  useEffect(() => {
    let active = true;
    void load()
      .then((data) => {
        if (active) {
          setQueryState({ key, data, error: null });
          setIsRetrying(false);
        }
      })
      .catch((error: unknown) => {
        if (active) {
          setQueryState({
            key,
            data: null,
            error: error instanceof Error && error.message ? error.message : "Unable to load analytics.",
          });
          setIsRetrying(false);
        }
      });
    return () => {
      active = false;
    };
  }, [key, retryKey, load]);

  const retry = useCallback(() => {
    setIsRetrying(true);
    setRetryKey((current) => current + 1);
  }, []);
  const hasCurrentKey = queryState?.key === key;
  return {
    data: hasCurrentKey ? queryState?.data ?? null : null,
    isLoading: !hasCurrentKey || isRetrying,
    error: hasCurrentKey ? queryState?.error ?? null : null,
    retry,
  };
}

export function useCompanyActivity(range: AnalyticsRange) {
  const load = useCallback(() => getCompanyActivity(range), [range]);
  return useAnalyticsQuery<CompanyActivityAnalytics>(range, load);
}

export function useMyActivity(range: AnalyticsRange) {
  const load = useCallback(() => getMyActivity(range), [range]);
  return useAnalyticsQuery<MyActivityAnalytics>(range, load);
}

export function usePending() {
  return useAnalyticsQuery<PendingAnalytics>("current", getPending);
}
