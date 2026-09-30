declare global {
  interface Window {
    /** 由服务端注入的对外公开地址（可能带反向代理的路径前缀），如 https://panel.example.com/7f3a9c2b */
    __RF_PUBLIC_BASE__?: string;
  }
}

/**
 * 拼接对外可用的公开地址。
 *
 * 面板挂在随机路径下时，复制按钮如果只用 window.location.origin + 路径，
 * 复制出来的订阅 / 规则集 / 转换链接会缺路径前缀而无法访问。
 * 服务端会把带前缀的公开地址注入到 window.__RF_PUBLIC_BASE__，这里统一使用它。
 */
export function publicUrl(path: string): string {
  const base = (window.__RF_PUBLIC_BASE__ ?? "").replace(/\/+$/, "");
  const suffix = path.startsWith("/") ? path : `/${path}`;
  return `${window.location.origin}${base}${suffix}`;
}
