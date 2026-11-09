$Host.UI.RawUI.WindowTitle = 'Restic Rubric Review'
$Host.UI.RawUI.BackgroundColor = 'Black'
$Host.UI.RawUI.ForegroundColor = 'White'
Clear-Host
Get-Content -Encoding UTF8 'F:\swe\099934\project-01\evidence\review\rubric-comparison.txt' | ForEach-Object {
 if ($_ -like 'Rubric ID：5*' -or $_ -like '*CANCEL source=*') { Write-Host $_ -ForegroundColor Yellow }
 else { Write-Host $_ }
}
@{Width=$Host.UI.RawUI.WindowSize.Width;Height=$Host.UI.RawUI.WindowSize.Height;CursorY=$Host.UI.RawUI.CursorPosition.Y} | ConvertTo-Json | Set-Content -Encoding UTF8 'F:\swe\099934\project-01\evidence\review\terminal-metrics.json'
while ($true) { Start-Sleep -Seconds 30 }
