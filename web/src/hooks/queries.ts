import { type QueryClient, useQueries, useQuery } from "@tanstack/react-query"

import { api, type SessionInfo, type VM } from "@/lib/api"
import type { Listing } from "@/lib/sessions"

/** How often the VM list and each session list reload. */
const REFRESH_MS = 10_000

const keys = {
  vms: ["vms"],
  sessions: (vm: string) => ["sessions", vm],
  models: (vm: string) => ["models", vm],
}

export function useVMs() {
  return useQuery({
    queryKey: keys.vms,
    queryFn: api.vms,
    refetchInterval: REFRESH_MS,
  })
}

async function listing(vm: string): Promise<Listing> {
  const list = await api.sessions(vm)
  return { sessions: list.sessions, live: list.live, states: list.states ?? {} }
}

/** The session lists of the named VMs, and the error of each list that failed, by VM. */
export function useListings(vms: string[]) {
  return useQueries({
    queries: vms.map((vm) => ({
      queryKey: keys.sessions(vm),
      queryFn: () => listing(vm),
      refetchInterval: REFRESH_MS,
      // Reloads in a hidden tab too, because the tab title marks a waiting approval.
      refetchIntervalInBackground: true,
    })),
    combine: (results) => {
      const lists: Record<string, Listing> = {}
      const errors: Record<string, Error> = {}
      results.forEach((result, i) => {
        if (result.data) lists[vms[i]] = result.data
        if (result.error) errors[vms[i]] = result.error
      })
      return { lists, errors }
    },
  })
}

/**
 * One VM from the VM list. `missing` is true once the list has loaded without it. `ready` is true for a running VM with
 * agent-manager.
 */
export function useVM(name: string) {
  const { data: vms, error } = useVMs()
  const vm = vms?.find((v) => v.name === name)
  const stopped = vm !== undefined && vm.status !== "running"
  return {
    vm,
    error,
    missing: vms !== undefined && !vm,
    stopped,
    noAgentManager: vm !== undefined && !stopped && !vm.agentManager,
    ready: vm !== undefined && !stopped && vm.agentManager,
  }
}

/** Running VMs with agent-manager. */
const listed = (vms: VM[] = []) =>
  vms.filter((v) => v.status === "running" && v.agentManager).map((v) => v.name)

/** The session lists of every running VM with agent-manager. VM rows and the tab title show what waits on you. */
export function useAllListings() {
  return useListings(listed(useVMs().data))
}

/** The models the VM's Claude offers. A VM's list loads once, and Retry loads it again. */
export function useModels(vm: string | undefined) {
  return useQuery({
    queryKey: keys.models(vm ?? ""),
    queryFn: () => api.models(vm ?? "").then((r) => r.models),
    enabled: vm !== undefined,
    staleTime: Infinity,
  })
}

/** Reloads the VM list and the session lists, after something on this page changed them. */
export function refreshLists(client: QueryClient) {
  void client.invalidateQueries({ queryKey: keys.vms })
  void client.invalidateQueries({ queryKey: ["sessions"] })
}

/** Reloads every query, for Retry. */
export function retryAll(client: QueryClient) {
  void client.invalidateQueries()
}

/** Shows a session made on this page in its VM's list before the list reloads. */
export function addCreated(client: QueryClient, vm: string, info: SessionInfo) {
  client.setQueryData<Listing>(keys.sessions(vm), (old) =>
    old ? { ...old, sessions: [info, ...old.sessions] } : old,
  )
  void client.invalidateQueries({ queryKey: keys.sessions(vm) })
}
