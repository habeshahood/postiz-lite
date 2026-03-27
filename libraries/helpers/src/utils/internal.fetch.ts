import { cookies, headers } from 'next/headers';
import { customFetch } from '@gitroom/helpers/utils/custom.fetch.func';

export const internalFetch = async (url: string, options: RequestInit = {}) => {
  const cookieStore = await cookies();
  const headerStore = await headers();
  const host = headerStore.get('host') || '';

  return customFetch(
    { baseUrl: process.env.BACKEND_INTERNAL_URL! },
    cookieStore?.get('auth')?.value!,
    cookieStore?.get('showorg')?.value!
  )(url, {
    ...options,
    headers: {
      ...(options?.headers || {}),
      'x-forwarded-host': host,
    },
  });
};
