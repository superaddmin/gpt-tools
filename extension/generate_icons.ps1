Add-Type -AssemblyName System.Drawing

$sizes = @(16, 48, 128)
$iconsDir = "f:\chatadd\extension\icons"

foreach ($size in $sizes) {
    $bmp = New-Object System.Drawing.Bitmap($size, $size)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.SmoothingMode = 'HighQuality'

    $cx = $size / 2
    $cy = $size / 2
    $r = $size * 0.42

    $greenBrush = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(255, 16, 163, 127))
    $g.FillEllipse($greenBrush, $cx - $r, $cy - $r, $r * 2, $r * 2)
    $greenBrush.Dispose()

    $whiteBrush = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::White)
    $penWidth = [Math]::Max(1.5, $size * 0.05)
    $whitePen = New-Object System.Drawing.Pen([System.Drawing.Color]::White, $penWidth)
    $whitePen.StartCap = 'Round'
    $whitePen.EndCap = 'Round'

    $keyW = $size * 0.28
    $keyH = $size * 0.12
    $keyX = $cx - $keyW / 2
    $keyY = $cy - $keyH / 2 - $size * 0.06
    $g.FillEllipse($whiteBrush, $keyX, $keyY, $keyW, $keyH)

    $toothW = $size * 0.06
    $toothH = $size * 0.08
    $g.FillRectangle($whiteBrush, $keyX + $keyW - $toothW * 0.8, $keyY + $keyH, $toothW, $toothH)
    $g.FillRectangle($whiteBrush, $keyX + $keyW - $toothW * 2.2, $keyY + $keyH, $toothW, $toothH * 0.7)

    $ringR = $size * 0.08
    $ringX = $keyX + $keyW * 0.15 - $ringR
    $ringY = $cy - $size * 0.06 - $ringR
    $g.DrawEllipse($whitePen, $ringX, $ringY, $ringR * 2, $ringR * 2)

    $arrowX = $cx
    $arrowY = $cy + $size * 0.14
    $arrowSize = $size * 0.1
    $g.DrawLine($whitePen, $arrowX, $arrowY - $arrowSize, $arrowX, $arrowY + $arrowSize * 0.6)
    $g.DrawLine($whitePen, $arrowX - $arrowSize * 0.7, $arrowY, $arrowX, $arrowY + $arrowSize * 0.8)
    $g.DrawLine($whitePen, $arrowX + $arrowSize * 0.7, $arrowY, $arrowX, $arrowY + $arrowSize * 0.8)

    $whiteBrush.Dispose()
    $whitePen.Dispose()
    $g.Dispose()

    $path = Join-Path $iconsDir "icon$size.png"
    $bmp.Save($path, [System.Drawing.Imaging.ImageFormat]::Png)
    $bmp.Dispose()
    Write-Host "Generated: $path"
}

Write-Host "All icons generated successfully!"
