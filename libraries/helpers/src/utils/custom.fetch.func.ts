export interface Params {
  baseUrl: string;
  beforeRequest?: (url: string, options: RequestInit) => Promise<RequestInit>;
  afterRequest?: (
    url: string,
    options: RequestInit,
    response: Response
  ) => Promise<boolean>;
}
export const customFetch = (
  params: Params,
  auth?: string,
  showorg?: string,
  secured: boolean = true
) => {
  return async function newFetch(url: string, options: RequestInit = {}) {
    // Read loggedAuth from URL on first load, then persist in window global
    // so it survives client-side navigations that drop the query param
    let loggedAuth: string | null | undefined;
    if (typeof window !== 'undefined') {
      const fromUrl = new URL(window.location.href).searchParams.get('loggedAuth');
      if (fromUrl) (window as any).__POSTIZ_AUTH = fromUrl;
      loggedAuth = (window as any).__POSTIZ_AUTH || fromUrl;
    }
    const newRequestObject = await params?.beforeRequest?.(url, options);
    const authNonSecuredCookie =
      typeof document === 'undefined'
        ? null
        : ((typeof window !== 'undefined' && (window as any).__POSTIZ_AUTH)
          || document.cookie
            .split(';')
            .find((p) => p.includes('auth='))
            ?.split('=')[1]
          || (typeof localStorage !== 'undefined' ? localStorage.getItem('auth') : null));

    const authNonSecuredOrg =
      typeof document === 'undefined'
        ? null
        : document.cookie
            .split(';')
            .find((p) => p.includes('showorg='))
            ?.split('=')[1];

    const authNonSecuredImpersonate =
      typeof document === 'undefined'
        ? null
        : document.cookie
            .split(';')
            .find((p) => p.includes('impersonate='))
            ?.split('=')[1];

    if (typeof window !== 'undefined' && url.includes('/user/self')) {
      console.log('[POSTIZ-DEBUG] loggedAuth:', loggedAuth ? loggedAuth.substring(0, 20) + '...' : 'NULL');
      console.log('[POSTIZ-DEBUG] cookieAuth:', authNonSecuredCookie ? String(authNonSecuredCookie).substring(0, 20) + '...' : 'NULL');
      console.log('[POSTIZ-DEBUG] location:', window.location.href.substring(0, 80));
    }
    const fetchRequest = await fetch(params.baseUrl + url, {
      ...(secured ? { credentials: 'include' } : {}),
      ...(newRequestObject || options),
      headers: {
        ...(showorg
          ? { showorg }
          : authNonSecuredOrg
          ? { showorg: authNonSecuredOrg }
          : {}),
        ...(options.body instanceof FormData
          ? {}
          : { 'Content-Type': 'application/json' }),
        Accept: 'application/json',
        ...options?.headers,
        ...(auth
          ? { auth }
          : authNonSecuredCookie
          ? { auth: authNonSecuredCookie }
          : {}),
        ...(loggedAuth ? { auth: loggedAuth } : {}),
        ...(authNonSecuredImpersonate
          ? { impersonate: authNonSecuredImpersonate }
          : {}),
      },
      // @ts-ignore
      ...(!options.next && options.cache !== 'force-cache'
        ? { cache: options.cache || 'no-store' }
        : {}),
    });

    if (
      !params?.afterRequest ||
      (await params?.afterRequest?.(url, options, fetchRequest))
    ) {
      return fetchRequest;
    }

    // @ts-ignore
    return new Promise((res) => {}) as Response;
  };
};

export const fetchBackend = customFetch({
  get baseUrl() {
    return process.env.BACKEND_URL!;
  },
});
