# Probe replays

A probe is a short game in which the player does one kind of thing, so the records it adds can be told apart. Exact timing is not needed: the decoder finds the frames itself, and the snapshot after each action shows its effect.

## Recording

- Sandbox on Abyss with Unlimited Metal, Unlimited Energy, Fast Build and Unlock All Weapons, so nothing waits on resources or build time. A probe that needs a commander is a skirmish against the Easy AI with the same options. The AI's own actions are not recorded; end the probe before it attacks.
- Under two minutes, one kind of action per probe, a few seconds apart.
- Keep the mouse still between actions where possible: every pointer move writes a record.
- Note each action in order.
- The game prunes old replays from `users/<steamid>/replays/`, so copy each one out soon.

A probe under 1 MiB is committed as `testdata/fixtures/<game version>/probe-<name>.fwr`, with its note as `<name>.md` in this directory. The note gives the frame of each action's records, from `fwr dump`.

## Probes

| Name | What it shows |
|---|---|
| delete | Strut pairs (0xcb, 0xcd), placement (0xd2), strut delete (0xce), scrap (0xd6) |
| node | Node delete (0xcc) |
| doors | Door material, opening and closing (0xd1) |
| mine | Placement on the ground: nodes 0, the orientation angle |
| ground | A strut that anchors to terrain (0xcb byte 66) |
| repair | Fire (0xca), repair (0xd0), the mortar's initial aim angle |
| dblclick | Immediate fire (0xc5, 0xc6), a blocked line of fire (0xc7) |
| downgrade-overdrive | Upgrade and downgrade (0xd5), with the Overdrive commander |
| move-phantom | Moving a device (0xd2 i32@59), with the Phantom commander |

The commander ability (0xda) takes too long to charge for a fixture under 1 MiB; `TestAbility` in `msg` pins its record. Camera moves, zoom, pause, weapon facing, aim and power keys, laser sights and clicks the game ignores write no record.

## Note template

    # <name>

    Game v<version>, <map>, <sandbox or skirmish and its options>.

    | Frame | Action | Records |
    |---|---|---|
    | 354 | ... | ... |
