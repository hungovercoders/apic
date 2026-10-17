# Working notes

What is here is for people and agents working on apic, not for people using
it. The user documentation is the site, [apic.sh](https://apic.sh/), whose
source is `website/`. `AGENTS.md` at the root carries what most tasks need;
this is the map of everything else.

| When you are… | Do this |
| --- | --- |
| changing the runner, adding a directive, a step or a command, or touching `--json` or an exit code | Read [architecture.md](architecture.md): the package map, the request lifecycle and the three contracts; the one place both are described |
| editing the site, the course, a lesson, the skill, a generated page or any prose check | Read [docs-site.md](docs-site.md): the Astro layout, the link contract, the deploys, the generators and the prose checks |
| changing a workflow, a linter setting, adding a `//nolint`, or fixing a failing slopstopper check | Read [ci.md](ci.md): what CI runs, why each linter is on, and the quality suite's config |
| adding a dependency, or touching a licence or notices file | Read [licensing.md](licensing.md): the permissive-only rule and the notices generator |
| cutting a release, or changing the installers, the packages, the image, the cask or the extension's release | Read [releasing.md](releasing.md): signing, SBOMs, the archive names three installers depend on, and the moving `v0` tag |
| writing a lesson for the course | Read [learn/_template.md](learn/_template.md): the template every lesson is copied from and the rules it follows |
| cutting a lesson's YouTube episode | Read [learn/_episode-template.md](learn/_episode-template.md): the title, thumbnail and description conventions |
| recording the course's episode b-roll or the README's animation | Read the VHS scripts in `learn/tapes/` and `assets/demo.tape`; `task tapes` records them all against the demo API and `task tapes:check` validates them |
| building a lesson's episode video, or changing what one shows or says | Read its shot list in `learn/episodes/NN.yaml` and `scripts/episodes/main.go`; `task episodes` builds every episode into `dist/episodes`, bar the voice, and `task episodes:check` validates the shot lists and tapes |
| relaxing the Content-Security-Policy for one path, or answering a CSP finding from DAST | Read [security/CSP_EXCEPTIONS.md](security/CSP_EXCEPTIONS.md): the baseline policy and the entry a per-path relaxation needs |
| changing the prose style | Read `.vale/`, the styles Vale holds the docs to (`task docs:check`); `docs-site.md` says where a term or a deliberate typo goes |
