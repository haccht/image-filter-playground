# Synthetic codec variants

These images contain generated noise or RGBA gradients, never user photographs.
They are embedded only in the resource harness, not the deployed component.
The authoritative encoded bytes are identified by manifest.json; normal tests
and resource checks use them without external dependencies.

To regenerate with the tools already present during validation:

```sh
python3 scripts/resources/testdata/generate.py
```

Generation was executed with Python 3 standard library, cjpeg (libjpeg-turbo)
3.2.1 and ImageMagick 7.1.2-35. The generator supplies deterministic raw pixels,
uses integer DCT and quality 90 for progressive JPEG, and writes 8/16-bit RGBA
Adam7 PNG with ancillary metadata excluded. Tool/version differences may change
encoded bytes; review regenerated files and their manifest together.

Go tests verify hashes, encoded sizes, image dimensions, progressive SOF2 and
multiple scans, 4:2:0/4:4:4 sampling and Adam7 RGBA depth/interlace fields.
