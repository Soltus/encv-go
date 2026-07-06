import { useI18n } from "@encv/shared-components/composables/useI18n";

export function formatDateTime(isoStr: string | undefined | null): string {
  if (!isoStr) return "";
  try {
    const d = new Date(isoStr);
    if (Number.isNaN(d.getTime())) return "";
    const { getLocale } = useI18n();
    const locale = getLocale();
    return new Intl.DateTimeFormat(locale, {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    }).format(d);
  } catch {
    return "";
  }
}

export function formatDuration(ms: number): string {
  if (ms < 0) return "";
  const totalSec = Math.floor(ms / 1000);
  if (totalSec < 60) return `${totalSec}s`;
  const min = Math.floor(totalSec / 60);
  const sec = totalSec % 60;
  if (min < 60) return `${min}m${sec}s`;
  const hr = Math.floor(min / 60);
  const rm = min % 60;
  return `${hr}h${rm}m`;
}
