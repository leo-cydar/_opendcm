
# OpenDCM Official Test Data

## Introduction
This folder hosts data used for both formal and informal verification of the OpenDCM set of softwares.

Contributors are requested to note the following two points:
- The test data directory shall **never** contain any Patient Identifiable Data (PID). _DICOM_ files that have not been anonymised as per the _DICOM_ standard, e.g in [_Annex_ E of PS3.15](http://dicom.nema.org/dicom/2013/output/chtml/part15/chapter_E.html) must be used with caution.
- The attribution section is to be updated with both the origin and relevant licenses of any files included. Origin can be for example a URL for online sources, or a physical address of the hospital from which the data was acquired.

## Directory Structure

#### File names
It is suggested that the file names of those _DICOMs_ containing real data to follow the convention: `<<SeriesInstanceUID>>.dcm`, to allow for sensible searching of the directory. In the case that multiple _DICOMs_ are to be used for the same `SeriesInstanceUID`, please name the file as follows: `<<SeriesInstanceUID>>_<<Number/ShortTag>>.dcm`

#### Synthetic data
Any folders named `synthetic` contain synthesised data based on the contents contained within the parent folder. In the case of the top-level `synthetic` folder, the contents are entirely generated and not based on any external source.

The contents of any files contained within a `synthetic` directory are _not_ intended to be representative of the original data, as they may for instance have been purposefully corrupted to test data parsing.
#### Restrictions
If there are any restrictive usage policies, please only include the data if both appropriate and absolutely necessary _(else, create a synthesised alternative)_, and create a file named `USAGE` within the directory detailing the restrictions.

## Attribution

|Directory|Origin|License|
|--|--|--|
|`TCIA/`|The Cancer Imaging Archive (TCIA)| Creative Commons Attribution 3.0 Unported License|

