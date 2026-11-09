param([switch]$UsePrintWindow)
Add-Type -AssemblyName System.Drawing
Add-Type @'
using System;
using System.Runtime.InteropServices;
public class ReviewWin {
 [StructLayout(LayoutKind.Sequential)] public struct RECT { public int L,T,R,B; }
 [StructLayout(LayoutKind.Sequential)] public struct POINT { public int X,Y; }
 [DllImport("user32.dll")] public static extern bool GetClientRect(IntPtr h,out RECT r);
 [DllImport("user32.dll")] public static extern bool ClientToScreen(IntPtr h,ref POINT p);
 [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
 [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr h,IntPtr after,int x,int y,int cx,int cy,uint f);
 [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h,IntPtr dc,uint flags);
}
'@
$window=Get-Process WindowsTerminal | Where-Object MainWindowTitle -eq 'Restic Rubric Review' | Select-Object -First 1
if(-not $window){throw 'Report terminal not found'}
$handle=$window.MainWindowHandle
[ReviewWin]::SetWindowPos($handle,[IntPtr](-1),100,30,0,0,0x0001 -bor 0x0040)|Out-Null
[ReviewWin]::SetForegroundWindow($handle)|Out-Null
Start-Sleep -Seconds 2
$rect=New-Object ReviewWin+RECT
[ReviewWin]::GetClientRect($handle,[ref]$rect)|Out-Null
$point=New-Object ReviewWin+POINT
[ReviewWin]::ClientToScreen($handle,[ref]$point)|Out-Null
$cw=$rect.R;$ch=$rect.B
$metrics=Get-Content -Raw -Encoding UTF8 'F:\swe\099934\project-01\evidence\review\terminal-metrics.json'|ConvertFrom-Json
$top=40;$left=8
$rowHeight=($ch-$top-8)/$metrics.Height
$width=$cw-20;$height=[Math]::Min($ch-$top-8,[Math]::Ceiling(($metrics.CursorY+1)*$rowHeight))
$output='F:\swe\099934\project-01\evidence\screenshots\01-rubric-comparison.png'
New-Item -ItemType Directory -Force -Path (Split-Path $output)|Out-Null
if($UsePrintWindow){
 $full=New-Object System.Drawing.Bitmap($cw,$ch)
 $g=[System.Drawing.Graphics]::FromImage($full);$dc=$g.GetHdc()
 try{if(-not [ReviewWin]::PrintWindow($handle,$dc,3)){throw 'PrintWindow returned false'}}finally{$g.ReleaseHdc($dc);$g.Dispose()}
 $crop=New-Object System.Drawing.Rectangle($left,$top,$width,$height)
 $bmp=$full.Clone($crop,$full.PixelFormat);$full.Dispose()
}else{
 $bmp=New-Object System.Drawing.Bitmap($width,$height)
 $g=[System.Drawing.Graphics]::FromImage($bmp)
 $g.CopyFromScreen($point.X+$left,$point.Y+$top,0,0,$bmp.Size)
 $g.Dispose()
}
$bmp.Save($output,[System.Drawing.Imaging.ImageFormat]::Png);$bmp.Dispose()
@{method=$(if($UsePrintWindow){'PrintWindow client region'}else{'CopyFromScreen window region'});x=$point.X+$left;y=$point.Y+$top;width=$width;height=$height;clientWidth=$cw;clientHeight=$ch;handle=[long]$handle} | ConvertTo-Json | Set-Content -Encoding UTF8 'F:\swe\099934\project-01\evidence\review\capture-metadata.json'
Get-Content 'F:\swe\099934\project-01\evidence\review\capture-metadata.json'
