import { onMounted, onUnmounted } from "vue";
import { isNativePluginMode, lockScreenOrientation, unlockScreenOrientation } from "@/plugins/SimVerse";

export function useScreenOrientation(type: "landscape-primary" | "portrait-primary" = "portrait-primary") {
  onMounted(() => {
    lockScreenOrientation(type).catch((error: unknown) => {
      console.warn("Failed to lock screen orientation:", error);
    });
  });

  onUnmounted(() => {
    if (isNativePluginMode()) {
      unlockScreenOrientation().catch(() => {});
    }
  });
}
