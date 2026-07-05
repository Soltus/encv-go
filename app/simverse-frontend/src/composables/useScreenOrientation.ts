import { onMounted, onUnmounted } from 'vue';
import { ScreenOrientation } from '@capacitor/screen-orientation';

export function useScreenOrientation(type: 'landscape-primary' | 'portrait-primary' = 'portrait-primary') {
  onMounted(() => {
    ScreenOrientation.lock({
      orientation: type,
    }).catch((error: unknown) => {
      console.warn('Failed to lock screen orientation:', error);
    });
  });
  
  onUnmounted(() => {
    // 组件卸载时不解锁，由父组件管理
  });
}
