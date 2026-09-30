declare global {
  interface Window {
    /** 由服务端注入的对外公开地址，可能是绝对地址（https://panel.example.com/7f3a9c2b）或仅路径前缀（/7f3a9c2b） */
    __RF_PUBLIC_BASE__?: string;
  }
}

/**
 * 拼接对外可用的公开地址。
 *
 * 面板挂在随机路径下时，复制按钮如果只用 window.location.origin + 路径，
 * 复制出来的订阅 / 规则集 / 转换链接会缺路径前缀而无法访问；
 * 服务端会把带前缀的公开地址注入 window.__RF_PUBLIC_BASE__。
 *
 * 注入值既可能是绝对地址（含 scheme 与 host），也可能只是路径前缀，
 * 这里统一处理：绝对地址直接用，否则拼当前 origin，避免出现 `https://hosthttps://host/...` 这种重复拼接。
 */
export function publicUrl(path: string): string {
  const raw = (window.__RF_PUBLIC_BASE__ ?? "").trim();
  const suffix = path.startsWith("/") ? path : `/${path}`;

  if (/^[a-z][a-z0-9+.-]*:\/\//i.test(raw)) {
    return `${raw.replace(/\/+$/, "")}${suffix}`;
  }

  const base = raw.replace(/\/+$/, "");
  return `${window.location.origin}${base}${suffix}`;
}
