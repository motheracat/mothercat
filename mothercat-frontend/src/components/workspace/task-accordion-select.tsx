"use client";

import { GlassSelect } from "@/components/workspace/glass-select";
import type { GlassSelectOption } from "@/components/workspace/glass-select";

export type TaskAccordionSelectOption = GlassSelectOption;

type Props = {
  label: string;
  value: string;
  options: TaskAccordionSelectOption[];
  onChange: (value: string) => void;
  placeholder: string;
  emptyMessage?: string;
  name?: string;
  disabled?: boolean;
};

export function TaskAccordionSelect(props: Props) {
  return <GlassSelect {...props} />;
}
