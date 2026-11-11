# Input: `tailscale debug netmap`. $s: `tailscale status --json`. $login: the owner's login.
# Prints ok, none, "open" with the extra ports and sources, or unknown, for the packet filter rules that reach this node.
def host: sub("/(32|128)$"; "");
($s.Self.TailscaleIPs) as $self
| [$s.User[] | select(.LoginName == $login) | .ID] as $ids
| [$s.Peer[]? | select(.UserID as $u | any($ids[]; . == $u)) | .TailscaleIPs[]] as $mine
| def toself: .Net as $n | ($n == "*" or $n == "0.0.0.0/0" or $n == "::/0" or $n == "100.64.0.0/10"
    or $n == "fd7a:115c:a1e0::/48" or any($self[]; . == ($n | host)));
  if has("PacketFilter") | not then "unknown"
  else
    [.PacketFilter[]? | . as $rule | .Dsts[] | select(toself) | {srcs: $rule.Srcs, first: .Ports.First, last: .Ports.Last}] as $in
    | if any($in[]; .first <= 443 and .last >= 443) | not then "none"
      else
        ([$in[] | select(.first != 443 or .last != 443) | "\(.first)-\(.last)"] | unique) as $ports
        | ([$in[] | .srcs[] | select(host as $h | any($mine[]; . == $h) | not)] | unique) as $others
        | if ($ports + $others | length) == 0 then "ok"
          else "open " + ([if $ports != [] then "ports \($ports | join(","))" else empty end,
            if $others != [] then "from \($others | join(","))" else empty end] | join("; ")) end
      end
  end
