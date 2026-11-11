"use client";

import { useCallback, useEffect, useState } from "react";

import { getCompanyActivity, getMyActivity, getPending } from "@/lib/analytics/api";
import type {
  AnalyticsRange,
  CompanyActivityAnalytics,
  MyActivityAnalytics,
  PendingAnalytics,
} from "@/lib/analytics/types";

type AnalyticsResponse = { range: AnalyticsRange };
type QueryState<T> = { range: AnalyticsRange; data: T | null; error: string | null } | null;
type AnalyticsResult<T> = { data: T | null; isLoading: boolean; error: string | null; retry: () => void };

function useAnalyticsQuery<T extends AnalyticsResponse>(
  range: AnalyticsRange,
  load: (selectedRange: AnalyticsRange) => Promise<T>,
): AnalyticsResult<T> {
  const [queryState, setQueryState] = useState<QueryState<T>>(null);
  const [retryKey, setRetryKey] = useState(0);
  const [isRetrying, setIsRetrying] = useState(false);

  useEffect(() => {
    let active = true;
    void load(range)
      .then((data) => {
        if (active) {
          setQueryState({ range, data, error: null });
          setIsRetrying(false);
        }
      })
      .catch((error: unknown) => {
        if (active) {
          setQueryState((current) => ({
            range,
            data: current?.data ?? null,
            error: error instanceof Error && error.message ? error.message : "Unable to load analytics.",
          }));
          setIsRetrying(false);
        }
      });
    return () => {
      active = false;
    };
  }, [range, retryKey, load]);

  const retry = useCallback(() => {
    setIsRetrying(true);
    setRetryKey((current) => current + 1);
  }, []);
  const hasCurrentRange = queryState?.range === range;
  return {
    data: queryState?.data ?? null,
    isLoading: !hasCurrentRange || isRetrying,
    error: hasCurrentRange ? queryState.error : null,
    retry,
  };
}

export function useCompanyActivity(range: AnalyticsRange) {
  return useAnalyticsQuery<CompanyActivityAnalytics>(range, getCompanyActivity);
}

export function useMyActivity(range: AnalyticsRange) {
  return useAnalyticsQuery<MyActivityAnalytics>(range, getMyActivity);
}

export function usePending(range: AnalyticsRange) {
  return useAnalyticsQuery<PendingAnalytics>(range, getPending);
}
