"use client";

import { GlassSelect } from "@/components/workspace/glass-select";
import type { GlassSelectOption } from "@/components/workspace/glass-select";

export type DepartmentOption = {
  id: string;
  name: string;
};

type Props = {
  label: string;
  value: string;
  departments: readonly DepartmentOption[];
  onChange: (value: string) => void;
  placeholder: string;
  emptyOptionLabel?: string;
  additionalOptions?: GlassSelectOption[];
  emptyMessage?: string;
  name?: string;
  disabled?: boolean;
};

export function DepartmentSelect({
  label,
  value,
  departments,
  onChange,
  placeholder,
  emptyOptionLabel,
  additionalOptions = [],
  emptyMessage,
  name,
  disabled,
}: Props) {
  const options = [
    ...(emptyOptionLabel === undefined ? [] : [{ value: "", label: emptyOptionLabel }]),
    ...additionalOptions,
    ...departments.map((department) => ({ value: department.id, label: department.name })),
  ];

  return (
    <GlassSelect
      label={label}
      value={value}
      options={options}
      onChange={onChange}
      placeholder={placeholder}
      emptyMessage={emptyMessage}
      name={name}
      disabled={disabled}
    />
  );
}
