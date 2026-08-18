from pathlib import Path

path = Path("src/benchharness/runner.go")
text = path.read_text(encoding="utf-8")

old = "\tchanges, diffErr := collectChanges(ctx, worktree)\n\tif diffErr != nil {"
new = "\tchanges, diffErr := collectChanges(ctx, worktree)\n\tif manifest.MetricsFile != \"\" {\n\t\tchanges = excludeBenchmarkChange(changes, manifest.MetricsFile)\n\t}\n\tif diffErr != nil {"
if old not in text:
    raise SystemExit("change collection anchor not found")
text = text.replace(old, new, 1)

old = '''\tcmd := exec.CommandContext(ctx, res.Command[0], res.Command[1:]...)
\tcmd.Dir = dir
\tcmd.Env = env
\tvar output bytes.Buffer
\tcmd.Stdout = &output
\tcmd.Stderr = &output
\tstarted := time.Now()
\terr := cmd.Run()
\tres.Duration = time.Since(started)
\tres.Output = truncate(output.String(), outputLimit)
\tif ctx.Err() == context.DeadlineExceeded {
\t\tres.TimedOut = true
\t\tres.ExitCode = -1
\t\treturn res
\t}
'''
new = '''\tcmd := exec.Command(res.Command[0], res.Command[1:]...)
\tcmd.Dir = dir
\tcmd.Env = env
\tprepareBenchmarkCommand(cmd)
\tvar output bytes.Buffer
\tcmd.Stdout = &output
\tcmd.Stderr = &output
\tstarted := time.Now()
\terr := cmd.Start()
\tif err == nil {
\t\tdone := make(chan error, 1)
\t\tgo func() { done <- cmd.Wait() }()
\t\tselect {
\t\tcase err = <-done:
\t\tcase <-ctx.Done():
\t\t\t_ = killBenchmarkCommandTree(cmd)
\t\t\terr = <-done
\t\t}
\t}
\tres.Duration = time.Since(started)
\tres.Output = truncate(output.String(), outputLimit)
\tif ctx.Err() == context.DeadlineExceeded {
\t\tres.TimedOut = true
\t\tres.ExitCode = -1
\t\treturn res
\t}
\tif ctx.Err() != nil {
\t\tres.ExitCode = -1
\t\tres.Output = truncate(strings.TrimSpace(res.Output+"\\n"+ctx.Err().Error()), outputLimit)
\t\treturn res
\t}
'''
if old not in text:
    raise SystemExit("run command anchor not found")
text = text.replace(old, new, 1)

anchor = '''func readAdapterMetrics(path string) (AdapterMetrics, error) {
'''
addition = '''func excludeBenchmarkChange(changes []FileChange, ignored string) []FileChange {
\tignored = filepath.ToSlash(filepath.Clean(strings.TrimSpace(ignored)))
\tif ignored == "" || ignored == "." {
\t\treturn changes
\t}
\tout := changes[:0]
\tfor _, change := range changes {
\t\tif filepath.ToSlash(filepath.Clean(change.Path)) == ignored {
\t\t\tcontinue
\t\t}
\t\tout = append(out, change)
\t}
\treturn out
}

'''
if addition not in text:
    if anchor not in text:
        raise SystemExit("metrics helper anchor not found")
    text = text.replace(anchor, addition + anchor, 1)

path.write_text(text, encoding="utf-8", newline="\n")
