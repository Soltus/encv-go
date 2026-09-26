/**
 * env.d.ts —— 补齐标准 lib 未声明的宿主 API。
 *
 * importScripts 只在经典 Worker 里存在，DOM lib 不声明它；
 * 而 SDK 的同一个 core.ts 既可能被 Worker 引入、也可能被主线程引入，
 * 因此这里统一声明，两处都能通过类型检查。
 */
declare function importScripts(...urls: string[]): void;
