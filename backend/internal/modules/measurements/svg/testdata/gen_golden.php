<?php

/*
 * Reference generator for the SVG golden files (TEC-298).
 *
 * This is the legacy hub's App\Services\Nexptg\NexptgSvgFillService
 * (olexfilms app/Services/Nexptg/NexptgSvgFillService.php) with the Laravel
 * collections and Eloquent models replaced by plain arrays; the DOM handling,
 * colors, averages and string building are copied unchanged, so the output
 * is what the PHP service produces for the same report.
 *
 * Usage (from this directory):
 *   php gen_golden.php fixture_sedan.json ../assets
 * writes sedan_composite_<place>.svg and sedan_part_<PART>.svg next to it.
 */

const COLORS = [
    -1 => '#9CA3AF',
    0 => '#e9df28',
    1 => '#55d37a',
    2 => '#deb50a',
    3 => '#ec4a08',
    4 => '#af0025',
    5 => '#af0025',
];

const PLACES = ['left', 'right', 'top', 'back'];

function fromValue(?int $v): int
{
    if ($v === null) {
        return -1;
    }

    return array_key_exists($v, COLORS) ? $v : -1;
}

function fillColor(int $v): string
{
    return COLORS[$v];
}

final class Fill
{
    public function __construct(
        private readonly string $root,
        private readonly array $detail,
        private readonly array $measurements,
    ) {}

    private function svgContents(string $part): string
    {
        $path = $this->root.'/'.$part.'.svg';
        if (! is_file($path)) {
            throw new InvalidArgumentException('NexPTG SVG not found: '.$part);
        }

        return file_get_contents($path);
    }

    public function fillPart(string $part): string
    {
        $svg = $this->svgContents($part);
        $byPosition = $this->measurementsByPosition($part);
        $avg = $this->averagePartInterpretation($part);
        $backgroundColor = $avg === null ? null : fillColor($avg);

        return $this->applyFills($svg, $part, $byPosition, $backgroundColor);
    }

    public function composePlaceView(string $place): ?string
    {
        $mainAsset = null;
        foreach ($this->detail['assets'] as $asset) {
            if (($asset['place'] ?? null) === $place && ($asset['kind'] ?? null) === 'main') {
                $mainAsset = $asset;
                break;
            }
        }
        if ($mainAsset === null) {
            return null;
        }
        $mainPart = (string) ($mainAsset['part'] ?? '');
        if ($mainPart === '') {
            return null;
        }
        try {
            $baseSvg = $this->svgContents($mainPart);
        } catch (InvalidArgumentException) {
            return null;
        }

        $partOverlaysXml = '';
        $pointGroupsXml = '';
        $radius = (float) ($this->detail['pointRadius'] ?? 22);

        foreach ($this->detail['assets'] as $asset) {
            if (($asset['place'] ?? null) !== $place || ($asset['kind'] ?? null) !== 'element') {
                continue;
            }
            $part = (string) ($asset['part'] ?? '');
            if ($part === '') {
                continue;
            }
            try {
                $filledPartSvg = $this->fillPart($part);
                $bodyXml = $this->extractPartBodyMarkup($filledPartSvg, $part);

                if ($bodyXml !== null && $bodyXml !== '') {
                    $partAttr = htmlspecialchars($part, ENT_QUOTES);
                    $partOverlaysXml .= <<<XML
    <g id="nexptg_part_body_{$partAttr}" data-part="{$partAttr}" opacity="0.72">
{$bodyXml}
    </g>

XML;
                }
            } catch (InvalidArgumentException) {
            }

            $byPosition = $this->measurementsByPosition($part);

            foreach ($asset['points'] ?? [] as $point) {
                $count = (int) ($point['count'] ?? 0);
                $x = $point['x'] ?? null;
                $y = $point['y'] ?? null;
                if ($count < 1 || $x === null || $y === null) {
                    continue;
                }
                $interpretation = $byPosition[$count] ?? -1;
                $color = fillColor($interpretation);
                $pointId = htmlspecialchars($part.'_point_'.$count, ENT_QUOTES);
                $partAttr = htmlspecialchars($part, ENT_QUOTES);
                $cx = htmlspecialchars((string) $x, ENT_QUOTES);
                $cy = htmlspecialchars((string) $y, ENT_QUOTES);
                $r = htmlspecialchars((string) $radius, ENT_QUOTES);
                $label = htmlspecialchars((string) $count, ENT_QUOTES);
                $fill = htmlspecialchars($color, ENT_QUOTES);

                $pointGroupsXml .= <<<XML
    <g id="{$pointId}" data-part="{$partAttr}" data-count="{$count}">
      <circle cx="{$cx}" cy="{$cy}" r="{$r}" fill="{$fill}" stroke="{$fill}" stroke-width="2"/>
      <text x="{$cx}" y="{$cy}" text-anchor="middle" dominant-baseline="central" font-family="Arial, Helvetica, sans-serif" font-size="19" font-weight="700" fill="#111827">{$label}</text>
    </g>

XML;
            }
        }

        if ($partOverlaysXml === '' && $pointGroupsXml === '') {
            return $baseSvg;
        }

        $overlay = '<g id="nexptg_measurement_overlay">'
            ."\n".$partOverlaysXml
            .($pointGroupsXml !== '' ? '    <g id="nexptg_measurement_points">'."\n".$pointGroupsXml.'    </g>'."\n" : '')
            .'  </g>';

        if (str_contains($baseSvg, '</svg>')) {
            return str_replace('</svg>', $overlay.'</svg>', $baseSvg);
        }

        return $baseSvg.$overlay;
    }

    private function extractPartBodyMarkup(string $filledSvg, string $part): ?string
    {
        $document = new DOMDocument;
        $previous = libxml_use_internal_errors(true);
        $loaded = $document->loadXML($filledSvg);
        libxml_clear_errors();
        libxml_use_internal_errors($previous);
        if (! $loaded) {
            throw new RuntimeException('regex fallback not expected for the shipped SVGs');
        }
        $xpath = new DOMXPath($document);
        $partNodes = $xpath->query(sprintf('//*[@id="%s"]', $part));
        if ($partNodes === false || $partNodes->length === 0) {
            return null;
        }
        $partNode = $partNodes->item(0);
        $markup = '';
        foreach ($partNode->childNodes as $child) {
            if (! $child instanceof DOMElement) {
                continue;
            }
            $childId = $child->getAttribute('id');
            if ($childId === $part.'_points' || str_contains($childId, '_point_')) {
                continue;
            }
            if (strtolower($child->tagName) === 'metadata') {
                continue;
            }
            $markup .= $document->saveXML($child)."\n";
        }

        return $markup !== '' ? $markup : null;
    }

    private function partMeasurements(string $part): array
    {
        return array_values(array_filter($this->measurements, fn (array $m): bool => (string) $m['part_type'] === $part));
    }

    public function averagePartInterpretation(string $part): ?int
    {
        $codes = [];
        foreach ($this->partMeasurements($part) as $m) {
            $code = $m['interpretation'] !== null ? (int) $m['interpretation'] : null;
            if ($code === null) {
                continue;
            }
            if (fromValue($code) === -1) {
                continue;
            }
            $codes[] = $code;
        }
        if ($codes === []) {
            return null;
        }
        $average = array_sum($codes) / count($codes);
        $rounded = (int) round($average);
        if ($rounded < 0) {
            $rounded = 0;
        }
        if ($rounded > 5) {
            $rounded = 5;
        }

        return fromValue($rounded);
    }

    private function measurementsByPosition(string $part): array
    {
        $out = [];
        foreach ($this->partMeasurements($part) as $m) {
            if ($m['position'] === null) {
                continue;
            }
            $out[(int) $m['position']] = fromValue($m['interpretation'] !== null ? (int) $m['interpretation'] : null);
        }

        return $out;
    }

    private function applyFills(string $svg, string $part, array $byPosition, ?string $backgroundColor = null): string
    {
        if ($byPosition === [] && $backgroundColor === null) {
            return $svg;
        }
        $document = new DOMDocument;
        $previous = libxml_use_internal_errors(true);
        $loaded = $document->loadXML($svg);
        libxml_clear_errors();
        libxml_use_internal_errors($previous);
        if (! $loaded) {
            throw new RuntimeException('regex fallback not expected for the shipped SVGs');
        }
        $xpath = new DOMXPath($document);
        if ($backgroundColor !== null) {
            $this->applyBackgroundFill($xpath, $part, $backgroundColor);
        }
        foreach ($byPosition as $position => $interpretation) {
            $pointId = $part.'_point_'.$position;
            $groups = $xpath->query(sprintf('//*[@id="%s"]', $pointId));
            if ($groups === false || $groups->length === 0) {
                continue;
            }
            $group = $groups->item(0);
            $circles = $group->getElementsByTagName('circle');
            for ($i = 0; $i < $circles->length; $i++) {
                $circle = $circles->item($i);
                $circle->setAttribute('fill', fillColor($interpretation));
                $circle->setAttribute('stroke', fillColor($interpretation));
            }
        }
        $result = $document->saveXML($document->documentElement);

        return $result !== false ? $result : $svg;
    }

    private function applyBackgroundFill(DOMXPath $xpath, string $part, string $color): void
    {
        $candidates = $xpath->query(sprintf(
            '//*[@id="%s"]//*[@fill="#FFFFFF" or @fill="#ffffff" or @fill="#FFF" or @fill="#fff" or @fill="white"]',
            $part
        ));
        if ($candidates === false) {
            return;
        }
        for ($i = 0; $i < $candidates->length; $i++) {
            $node = $candidates->item($i);
            if (! $node instanceof DOMElement) {
                continue;
            }
            if ($this->isInsidePointsGroup($node, $part)) {
                continue;
            }
            $node->setAttribute('fill', $color);
        }
    }

    private function isInsidePointsGroup(DOMElement $node, string $part): bool
    {
        $current = $node->parentNode;
        while ($current instanceof DOMElement) {
            $id = $current->getAttribute('id');
            if ($id === $part.'_points' || str_contains($id, '_point_')) {
                return true;
            }
            if ($id === $part) {
                return false;
            }
            $current = $current->parentNode;
        }

        return false;
    }
}

[$self, $fixturePath, $assets] = $argv + [null, null, null];
if ($fixturePath === null || $assets === null) {
    fwrite(STDERR, "usage: php gen_golden.php <fixture.json> <assets dir>\n");
    exit(2);
}
$fixture = json_decode(file_get_contents($fixturePath), true, 512, JSON_THROW_ON_ERROR);
$catalog = json_decode(file_get_contents($assets.'/catalog.json'), true, 512, JSON_THROW_ON_ERROR);
$model = null;
foreach ($catalog['models'] as $m) {
    if ($m['id'] === $fixture['body_type']) {
        $model = $m;
    }
}
if ($model === null) {
    fwrite(STDERR, "unknown body type\n");
    exit(1);
}
$root = $assets.'/'.$model['id'];
$detail = json_decode(file_get_contents($root.'/svg_manifest.json'), true, 512, JSON_THROW_ON_ERROR);
$fill = new Fill($root, $detail, $fixture['measurements']);
$name = $fixture['name'];
$dir = dirname($fixturePath);

foreach (PLACES as $place) {
    $svg = $fill->composePlaceView($place);
    if ($svg !== null) {
        file_put_contents("{$dir}/{$name}_composite_{$place}.svg", $svg);
    }
}
foreach ($fixture['parts'] ?? [] as $part) {
    file_put_contents("{$dir}/{$name}_part_{$part}.svg", $fill->fillPart($part));
}
