# Changelog

## [0.4.0](https://github.com/an0nfunc/forts-replay/compare/v0.3.1...v0.4.0) (2026-10-25)


### Features

* **match:** decode unknown header versions with the closest layout ([ea006da](https://github.com/an0nfunc/forts-replay/commit/ea006da0b41d889ec581bfb169bb510cccc21063))

## [0.3.1](https://github.com/an0nfunc/forts-replay/compare/v0.3.0...v0.3.1) (2026-10-23)


### Bug Fixes

* **fwr:** show SteamIDs in dump and hex as info does ([c6e4c44](https://github.com/an0nfunc/forts-replay/commit/c6e4c449c0988046e010185053bad6378942ee9f))
* **match:** end the summary at the first snapshot naming the winner ([7614c1e](https://github.com/an0nfunc/forts-replay/commit/7614c1e9a89bf27719c1956ba8bca1c61188de1f))

## [0.3.0](https://github.com/an0nfunc/forts-replay/compare/v0.2.0...v0.3.0) (2026-10-23)


### Features

* decode game v1.39.0 replays (header version 1390) ([e3a7e60](https://github.com/an0nfunc/forts-replay/commit/e3a7e6067b377156bb51e73e6c9faf77a3139283))

## [0.2.0](https://github.com/an0nfunc/forts-replay/compare/v0.1.0...v0.2.0) (2026-10-22)


### Features

* **fwr:** add fwr version and parser_version ([527fb2c](https://github.com/an0nfunc/forts-replay/commit/527fb2ce8f9a43809046db0b91a40242d6dc4742))
* **fwr:** show match state in info and dump ([235c4f8](https://github.com/an0nfunc/forts-replay/commit/235c4f8669143effe8903099a2bacd7de3fe19da))
* **match:** add economy, damage, losses and fort state to the summary ([a75ec16](https://github.com/an0nfunc/forts-replay/commit/a75ec167ac376bfb5b3d3c99b457fd06505354ed))
* **match:** add finished builds and the scrap rule for losses ([712c421](https://github.com/an0nfunc/forts-replay/commit/712c421522c27784794186b9d8ed666a11442afc))
* **state:** decode the snapshot world state ([4c46f74](https://github.com/an0nfunc/forts-replay/commit/4c46f74e2afb23cedede130bdb887d42044ab664))

## 0.1.0 (2026-10-22)


### Features

* **fwr:** add info --max-commands ([e0eff8b](https://github.com/an0nfunc/forts-replay/commit/e0eff8b22745440c6c797063d07cbfd1da866421))
* **match:** leave the game's weapon doors out of timelines ([49cbe4c](https://github.com/an0nfunc/forts-replay/commit/49cbe4c45cb3cb338a8e10fccb9751d508175d1e))
* **msg:** decode repairs, portals, aimed fire, ammo and script calls ([e8bb876](https://github.com/an0nfunc/forts-replay/commit/e8bb876299f510a24a102852b8f2c8905a736e00))
* **msg:** decode the team in player commands and door sources ([608f8a3](https://github.com/an0nfunc/forts-replay/commit/608f8a3b78de1947a3301d340968f4986fd3b7cb))
* **schema:** open the command kind list ([f9f9b7a](https://github.com/an0nfunc/forts-replay/commit/f9f9b7a185d120292deecf21190fa5c8fc540721))
* toolbox for Forts replay files ([a4b81d5](https://github.com/an0nfunc/forts-replay/commit/a4b81d56a60aede283dc926804b63e6720476097))
