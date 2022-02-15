#!/usr/bin/env python3


from typing import Dict
import requests
import json
import logging
import argparse
import sys

DEBIAN_URL = "https://security-tracker.debian.org/tracker/data/json"
TRIVY_IGNORE_PATH = ".trivyignore"
DISTRIBUTION = "bullseye"

def retrieve_debian_data() -> Dict:
    resp = requests.get(DEBIAN_URL)
    if resp.status_code != 200:
        print("Error while retrieving the data from {}".format(DEBIAN_URL))
        return None
    
    return json.loads(resp.content)
    

def read_trivy_filter(file_path) -> list:
    filtered_cve_list = list()
    with open(file_path, 'r') as file:
        for line in file:
            if not line.strip().startswith("CVE"):
                continue
            if line.strip() in filtered_cve_list:
                continue
            filtered_cve_list.append(line.strip())
    return filtered_cve_list


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument("--trivy-ignore", type=str, required=True)
    parser.add_argument("--distribution", type=str, required=True)

    args = parser.parse_args()

    logger = logging.getLogger("check_trivy")
    logger.setLevel(logging.INFO)
    ch = logging.StreamHandler()
    ch.setLevel(logging.DEBUG)
    formatter = logging.Formatter('%(asctime)s - %(name)s - %(levelname)s - %(message)s')
    ch.setFormatter(formatter)
    logger.addHandler(ch)


    resolved = False
    ignored_cves = read_trivy_filter(args.trivy_ignore)
    all_cves = retrieve_debian_data()
    if all_cves is None:
        sys.exit(1)
    for package in all_cves.keys():
        for cve in all_cves[package].keys():
            if cve in ignored_cves:
                try:
                    if args.distribution not in all_cves[package][cve]["releases"]:
                        logger.info("{} is not in {}".format(args.distribution, package))
                        continue
                    if all_cves[package][cve]["releases"][args.distribution]["status"] == "resolved":
                        logger.warning("{} has been resolved".format(cve))
                        resolved = True
                    else:
                        logger.debug("{} has been found but it is not resolved".format(cve))
                    ignored_cves.remove(cve)
                except KeyError:
                    logger.warning("Status for {} has not been found".format(cve))
    if resolved:
        sys.exit(1)
    elif len(ignored_cves) != 0:
        logger.error("These CVE has not been found: {}".format(ignored_cves))
        sys.exit(1)

                    
                
