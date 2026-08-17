'use client';

interface PreviewDurationSelectProps {
  value: number;
  onChange: (seconds: number) => void;
  disabled?: boolean;
}

const MAX_HOURS = 6;
const MAX_SECONDS = MAX_HOURS * 60 * 60;
const SELECT_CLASS_NAME = 'rounded-md border border-gray-300 bg-white px-2 py-1 text-sm text-gray-900 disabled:bg-gray-100 disabled:text-gray-400';

const clampDuration = (seconds: number) => Math.min(MAX_SECONDS, Math.max(1, Math.round(seconds || 0)));

export default function PreviewDurationSelect({
  value,
  onChange,
  disabled = false,
}: PreviewDurationSelectProps) {
  const normalizedValue = clampDuration(value);
  const hours = Math.floor(normalizedValue / 3600);
  const minutes = Math.floor((normalizedValue % 3600) / 60);
  const seconds = normalizedValue % 60;
  const minuteAndSecondLimit = hours === MAX_HOURS ? 1 : 60;

  const updateDuration = (nextHours: number, nextMinutes: number, nextSeconds: number) => {
    const total = nextHours === MAX_HOURS
      ? MAX_SECONDS
      : nextHours * 3600 + nextMinutes * 60 + nextSeconds;
    onChange(clampDuration(total));
  };

  return (
    <span className="inline-flex flex-wrap items-center gap-2">
      <select
        aria-label="试看小时"
        value={hours}
        disabled={disabled}
        onChange={(event) => updateDuration(Number(event.target.value), minutes, seconds)}
        className={SELECT_CLASS_NAME}
      >
        {Array.from({ length: MAX_HOURS + 1 }, (_, option) => (
          <option key={option} value={option}>{option}小时</option>
        ))}
      </select>
      <select
        aria-label="试看分钟"
        value={hours === MAX_HOURS ? 0 : minutes}
        disabled={disabled}
        onChange={(event) => updateDuration(hours, Number(event.target.value), seconds)}
        className={SELECT_CLASS_NAME}
      >
        {Array.from({ length: minuteAndSecondLimit }, (_, option) => (
          <option key={option} value={option}>{option}分</option>
        ))}
      </select>
      <select
        aria-label="试看秒"
        value={hours === MAX_HOURS ? 0 : seconds}
        disabled={disabled}
        onChange={(event) => updateDuration(hours, minutes, Number(event.target.value))}
        className={SELECT_CLASS_NAME}
      >
        {Array.from({ length: minuteAndSecondLimit }, (_, option) => (
          <option key={option} value={option}>{option}秒</option>
        ))}
      </select>
    </span>
  );
}
