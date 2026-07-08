import { onMounted, onUnmounted } from "vue";
import { lockScreenOrientation, unlockScreenOrientation, isNativePluginMode } from "@/plugins/SimVerse";

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
