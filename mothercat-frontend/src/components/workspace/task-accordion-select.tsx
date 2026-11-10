"use client";

import { Check, ChevronDown } from "lucide-react";
import { useId, useRef, useState, type KeyboardEvent } from "react";

import styles from "./task-accordion-select.module.css";

export type TaskAccordionSelectOption = {
  value: string;
  label: string;
};

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

export function TaskAccordionSelect({
  label,
  value,
  options,
  onChange,
  placeholder,
  emptyMessage = "No options available.",
  name,
  disabled = false,
}: Props) {
  const [open, setOpen] = useState(false);
  const panelId = useId();
  const triggerRef = useRef<HTMLButtonElement>(null);
  const selectedOption = options.find((option) => option.value === value);

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === "Escape" && open) {
      event.preventDefault();
      setOpen(false);
      triggerRef.current?.focus();
    }
  }

  return (
    <div className={styles.root} onKeyDown={handleKeyDown}>
      {name ? <input type="hidden" name={name} value={value} /> : null}
      <button
        ref={triggerRef}
        type="button"
        className={styles.trigger}
        aria-label={label}
        aria-expanded={open}
        aria-controls={open ? panelId : undefined}
        disabled={disabled}
        onClick={() => setOpen((current) => !current)}
      >
        <span className={styles.triggerCopy}>
          <span className={selectedOption ? styles.value : styles.placeholder}>
            {selectedOption?.label ?? placeholder}
          </span>
          <span className={styles.triggerHint}>{open ? "Choose an option" : "Select to change"}</span>
        </span>
        <ChevronDown
          size={18}
          aria-hidden="true"
          className={`${styles.chevron}${open ? ` ${styles.chevronOpen}` : ""}`}
        />
      </button>
      {open ? (
        <div id={panelId} className={styles.panel} role="group" aria-label={`${label} options`}>
          {options.length ? (
            <div className={styles.options}>
              {options.map((option) => {
                const selected = value === option.value;
                return (
                  <button
                    key={option.value}
                    type="button"
                    className={`${styles.option}${selected ? ` ${styles.optionSelected}` : ""}`}
                    aria-pressed={selected}
                    onClick={() => {
                      onChange(option.value);
                      setOpen(false);
                      triggerRef.current?.focus();
                    }}
                  >
                    <span>{option.label}</span>
                    {selected ? <Check size={17} aria-hidden="true" /> : null}
                  </button>
                );
              })}
            </div>
          ) : (
            <p className={styles.emptyMessage} role="status">{emptyMessage}</p>
          )}
        </div>
      ) : null}
    </div>
  );
}
