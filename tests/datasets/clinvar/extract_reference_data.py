#!/usr/bin/env python3
"""
Extract ClinVar reference data from NCBI XML files.

Downloads ClinVarVCVRelease XML and extracts data for test IDs.
Creates:
  - reference_data.json: Parsed variant information for test validation
  - reference_data_raw.xml: Original XML format (complete reference)
"""

import sys
import json
import gzip
import urllib.request
import xml.etree.ElementTree as ET
from pathlib import Path

# NCBI FTP URL (same as in biobtree config)
NCBI_FTP_BASE = "https://ftp.ncbi.nlm.nih.gov/pub/clinvar/xml/"
XML_FILE = "ClinVarVCVRelease_00-latest.xml.gz"
IDS_FILE = "clinvar_ids.txt"
OUTPUT_FILE = "reference_data.json"
OUTPUT_XML_FILE = "reference_data_raw.xml"
CACHE_FILE = "ClinVarVCVRelease_00-latest.xml.gz"  # Cache downloaded file


def download_clinvar_xml() -> str:
    """Download ClinVar VCV XML or use cached version"""
    cache_path = Path(CACHE_FILE)

    # Check if cached file exists and is recent (< 7 days old)
    if cache_path.exists():
        import time
        age_days = (time.time() - cache_path.stat().st_mtime) / 86400
        if age_days < 7:
            print(f"Using cached file: {CACHE_FILE} (age: {age_days:.1f} days)")
            return str(cache_path)

    url = NCBI_FTP_BASE + XML_FILE
    print(f"Downloading ClinVar VCV XML from {url}...")
    print("This may take several minutes (file is ~2-3 GB compressed)...")

    try:
        urllib.request.urlretrieve(url, CACHE_FILE)
        size_mb = cache_path.stat().st_size / (1024 * 1024)
        print(f"✓ Downloaded and cached ({size_mb:.1f} MB)")
        return str(cache_path)
    except Exception as e:
        print(f"Error downloading file: {e}")
        raise


def extract_variants_from_xml(xml_path: str, target_ids: set) -> tuple:
    """
    Extract and parse VariationArchive elements matching target IDs from ClinVar XML.

    Returns:
        tuple: (parsed_variants, raw_xml_elements)
            - parsed_variants: list of dicts with parsed variant information
            - raw_xml_elements: list of XML strings (original format)
    """
    print(f"Parsing {xml_path} for {len(target_ids)} target IDs...")
    print("This may take 10-20 minutes to scan the full file...")

    variants = []
    raw_xml_list = []
    found_ids = set()
    processed_count = 0

    with gzip.open(xml_path, 'rb') as f:
        # Use iterparse for memory-efficient streaming
        context = ET.iterparse(f, events=('start', 'end'))

        # Get root element for namespace handling
        event, root = next(context)

        for event, elem in context:
            if event == 'end' and elem.tag == 'VariationArchive':
                processed_count += 1

                # Progress logging
                if processed_count % 10000 == 0:
                    print(f"  Processed {processed_count} variations, found {len(found_ids)}/{len(target_ids)}...")

                # Check VariationID attribute
                variation_id = elem.get('VariationID')
                if variation_id and variation_id in target_ids and variation_id not in found_ids:
                    # Parse XML element into dict
                    variant_dict = parse_variation_archive(elem, variation_id)
                    if variant_dict:
                        variants.append(variant_dict)

                        # Convert XML element to string (raw format)
                        raw_xml = ET.tostring(elem, encoding='unicode')
                        raw_xml_list.append(raw_xml)

                        found_ids.add(variation_id)
                        print(f"  ✓ Found {variation_id}: {variant_dict.get('name', '')[:60]}")

                    # Early exit if we found all
                    if len(found_ids) == len(target_ids):
                        print(f"Found all {len(target_ids)} target variations, stopping scan")
                        break

                # Clear element to free memory
                elem.clear()
                root.clear()

    print(f"✓ Scanned {processed_count} variations total")
    return variants, raw_xml_list


def parse_variation_archive(elem: ET.Element, variation_id: str) -> dict:
    """Parse a VariationArchive XML element into a dict with key fields"""

    # Extract basic info from ClassifiedRecord
    classified_record = elem.find('.//ClassifiedRecord')
    if classified_record is None:
        return None

    # Get classifications
    classifications = classified_record.find('.//Classifications')
    germline_class = ""
    review_status = ""

    if classifications is not None:
        germline_list = classifications.find('.//GermlineClassification')
        if germline_list is not None:
            desc = germline_list.find('.//Description')
            if desc is not None and desc.text:
                germline_class = desc.text

            review_elem = germline_list.find('.//ReviewStatus')
            if review_elem is not None and review_elem.text:
                review_status = review_elem.text

    # Get simple allele info
    simple_allele = classified_record.find('.//SimpleAllele')
    variant_type = ""
    gene_list = []
    gene_ids = []
    hgnc_ids = []
    hgvs_list = []

    if simple_allele is not None:
        variant_type = simple_allele.get('VariantType', '')

        # Get gene annotations
        for gene in simple_allele.findall('.//Gene'):
            gene_symbol = gene.get('Symbol', '')
            if gene_symbol:
                gene_list.append(gene_symbol)
            gene_id = gene.get('GeneID', '')
            if gene_id:
                gene_ids.append(gene_id)
            hgnc_id = gene.get('HGNC_ID', '')
            if hgnc_id:
                hgnc_ids.append(hgnc_id)

        # Get HGVS expressions
        for hgvs in simple_allele.findall('.//HGVS'):
            nucleotide = hgvs.find('.//NucleotideExpression')
            if nucleotide is not None:
                expr = nucleotide.find('.//Expression')
                if expr is not None and expr.text:
                    hgvs_list.append(expr.text)

    # Get variant name
    variant_name = ""
    name_elem = simple_allele.find('.//Name') if simple_allele is not None else None
    if name_elem is not None and name_elem.text:
        variant_name = name_elem.text
    elif hgvs_list:
        variant_name = hgvs_list[0]  # Use first HGVS as name

    # Get location info (prefer GRCh38, fallback to GRCh37)
    location_grch38 = None
    location_grch37 = None

    if simple_allele is not None:
        for loc in simple_allele.findall('.//Location/SequenceLocation'):
            assembly = loc.get('Assembly', '')
            if assembly == 'GRCh38':
                location_grch38 = loc
            elif assembly == 'GRCh37':
                location_grch37 = loc

    location = location_grch38 if location_grch38 is not None else location_grch37

    chromosome = ""
    start = ""
    stop = ""
    assembly = ""
    reference_allele = ""
    alternate_allele = ""

    if location is not None:
        chromosome = location.get('Chr', '')
        start = location.get('start', '')
        stop = location.get('stop', '')
        assembly = location.get('Assembly', '')
        reference_allele = location.get('referenceAllele', '')
        alternate_allele = location.get('alternateAllele', '')

    # Get phenotypes
    trait_set = classified_record.find('.//TraitSet')
    phenotype_list = []
    phenotype_ids = []

    if trait_set is not None:
        for trait in trait_set.findall('.//Trait'):
            for name in trait.findall('.//Name/ElementValue'):
                if name.text:
                    phenotype_list.append(name.text)
                    break  # Take first name only

            # Get phenotype IDs (MedGen, OMIM, etc.)
            for xref in trait.findall('.//XRef'):
                db = xref.get('DB', '')
                xref_id = xref.get('ID', '')
                if db and xref_id:
                    phenotype_ids.append(f"{db}:{xref_id}")

    # Build variant dict
    variant = {
        "variation_id": variation_id,
        "name": variant_name,
        "type": variant_type,
        "germline_classification": germline_class,
        "review_status": review_status,
        "gene_symbols": gene_list,
        "gene_ids": gene_ids,
        "hgnc_ids": hgnc_ids,
        "hgvs_expressions": hgvs_list,
        "chromosome": chromosome,
        "start": start,
        "stop": stop,
        "assembly": assembly,
        "reference_allele": reference_allele,
        "alternate_allele": alternate_allele,
        "phenotype_list": phenotype_list,
        "phenotype_ids": phenotype_ids,
    }

    return variant


def main():
    """Main extraction process"""
    # Read target IDs
    ids_path = Path(IDS_FILE)
    if not ids_path.exists():
        print(f"Error: {IDS_FILE} not found")
        print("Run: cp ../../../test_out/reference/clinvar_ids.txt .")
        return 1

    with open(ids_path, 'r') as f:
        target_ids = set(line.strip() for line in f if line.strip())

    print(f"Target IDs: {len(target_ids)}")
    print(f"First 10: {sorted(list(target_ids))[:10]}")
    print()

    # Download/load XML file
    try:
        xml_path = download_clinvar_xml()
    except Exception as e:
        print(f"Error: {e}")
        return 1

    # Extract and parse matching variants
    variants, raw_xml_list = extract_variants_from_xml(xml_path, target_ids)

    print(f"\n✓ Extracted {len(variants)}/{len(target_ids)} variants")

    # Validate we found all IDs
    found_ids = {v['variation_id'] for v in variants}
    missing = target_ids - found_ids
    if missing:
        print(f"⚠ Warning: Missing {len(missing)} IDs:")
        for mid in sorted(list(missing))[:10]:
            print(f"  - {mid}")
        if len(missing) > 10:
            print(f"  ... and {len(missing) - 10} more")

    # Save parsed reference data as JSON
    output_path = Path(OUTPUT_FILE)
    with open(output_path, 'w', encoding='utf-8') as f:
        json.dump(variants, f, indent=2, ensure_ascii=False)

    file_size_kb = output_path.stat().st_size / 1024
    print(f"\n✓ Saved parsed data to {OUTPUT_FILE} ({file_size_kb:.1f} KB)")

    # Save raw XML elements
    output_xml_path = Path(OUTPUT_XML_FILE)
    with open(output_xml_path, 'w', encoding='utf-8') as f:
        # Write XML declaration and root element
        f.write('<?xml version="1.0" encoding="UTF-8"?>\n')
        f.write('<ClinVarReferenceData>\n')
        f.write('  <!-- Raw VariationArchive elements for test reference -->\n')
        f.write('  <!-- Total variations: {} -->\n\n'.format(len(raw_xml_list)))

        # Write each VariationArchive element with proper indentation
        for raw_xml in raw_xml_list:
            # Add indentation to each line
            indented = '\n'.join('  ' + line if line.strip() else line
                                 for line in raw_xml.split('\n'))
            f.write(indented)
            f.write('\n\n')

        f.write('</ClinVarReferenceData>\n')

    xml_size_kb = output_xml_path.stat().st_size / 1024
    print(f"✓ Saved raw XML to {OUTPUT_XML_FILE} ({xml_size_kb:.1f} KB)")

    # Show sample
    if variants:
        sample = variants[0]
        print(f"\nSample variant (showing extracted fields):")
        print(f"  Variation ID: {sample['variation_id']}")
        print(f"  Name: {sample['name'][:80]}...")
        print(f"  Type: {sample['type']}")
        print(f"  Germline Classification: {sample['germline_classification']}")
        print(f"  Review Status: {sample['review_status']}")
        print(f"  Genes: {', '.join(sample['gene_symbols'][:3])}")
        print(f"  HGVS: {len(sample['hgvs_expressions'])} expression(s)")
        print(f"  Location: chr{sample['chromosome']}:{sample['start']}-{sample['stop']} ({sample['assembly']})")
        print(f"  Alleles: {sample['reference_allele']} > {sample['alternate_allele']}")
        print(f"  Phenotypes: {len(sample['phenotype_list'])}")

    print(f"\n{'='*70}")
    print(f"✓ Reference data extraction complete")
    print(f"{'='*70}")
    print(f"  Target IDs: {len(target_ids)}")
    print(f"  Found: {len(found_ids)}")
    print(f"  Missing: {len(missing)}")
    print()
    print("Files created:")
    print(f"  - {OUTPUT_FILE:30s} : Parsed data for tests")
    print(f"  - {OUTPUT_XML_FILE:30s} : Original XML format (complete reference)")
    print()
    print("Next steps:")
    print("  1. Review reference_data.json and reference_data_raw.xml")
    print("  2. Run tests: python3 ../../run_tests.py clinvar")

    return 0


if __name__ == "__main__":
    sys.exit(main())
