# Header version 1390: open questions

`docs/format.md` describes header version 1390 (game v1.39.0) alongside 1383. Its samples are the 13 single-player replays of the corpus folder `v1.39.0/`, each repeating the scenario of the v1.38.2 corpus replay of the same name, and one lobby game outside the corpus. These points need more v1.39.0 recordings; the offline ones can repeat existing v1.38.2 corpus replays.

1. **Ranked:** one ranked 1v1, for the ranked block after the flag at 0x350, the ranked result's type (0xbe or 0xbf expected) and layout, the opponent's name in the player table, and the record that was 0x91 in 1383 ranked games. Once a sample places the result, its type goes into `msg.RecordType` and the parser decodes it like the 1383 record.
2. **Commands without a sample** (1383 IDs, expected 4 higher with the same layout): 0xcf and 0xd0 (as in `strut-repair.fwr`), 0xdc (as in `ammo-select.fwr`) and 0xc9.
3. **Which option adds the water cannon** to the weapon table: two sandbox games, one with and one without `weapon_pack`, each placing every weapon slot once, would tie it to an option and confirm the mortar at 5, the cannon at 23 and the ammunition IDs.
4. **The weapon field at +184:** 0 in every sample. A sandbox game that fires a water cannon, a laser and a firebeam, with snapshots during the beams, may show it set.
5. **Header bytes 21 to 24:** a replay with a value other than `00 00 00 00` or `02 00 00 00` there would tell a u8 and 3 bytes from one u32.
6. **Lobby records:** the 1383 0xb8 (near the end of a lobby game), 0xba and 0xe3, and chat the game writes itself (sender 0).
7. **The water cannon's 0xcc and aim records:** a sandbox game that fires a water cannon several times, keeps it selected and moves the pointer.
8. **Match start:** every v1.39.0 probe issues its first command about 500 frames later than its v1.38.2 twin, while the gaps between commands match. A scenario that acts at the first moment the match accepts input would tell whether v1.39.0 adds about 20 seconds before input.
