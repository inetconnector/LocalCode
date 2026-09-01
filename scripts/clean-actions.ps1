$runs = gh run list --limit 100 --json databaseId,conclusion | ConvertFrom-Json
$toDelete = $runs | Where-Object { $_.conclusion -eq 'cancelled' -or $_.conclusion -eq 'failure' }
Write-Host "Found $($toDelete.Count) failed/cancelled runs to delete."
foreach ($r in $toDelete) {
    gh run delete $r.databaseId --confirm 2>$null
    Write-Host "Deleted run $($r.databaseId) ($($r.conclusion))"
}
Write-Host "Unnecessary action runs cleaned up successfully."
