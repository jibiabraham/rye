import {
  type NavigateOptions,
  useLocation,
  useRouter,
} from "@tanstack/react-router"

/** History state for a link or push. It holds the URL the link or push leaves. */
export function useFrom() {
  return { from: useLocation({ select: (l) => l.href }) }
}

/** Goes to a parent page. Back, when the app came from there, and a replace otherwise. */
export function useUp() {
  const router = useRouter()
  return (to: NavigateOptions) => {
    if (router.state.location.state.from === router.buildLocation(to).href) {
      router.history.back()
    } else {
      void router.navigate({ ...to, replace: true })
    }
  }
}
