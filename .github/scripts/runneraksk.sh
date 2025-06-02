#!/bin/bash

az=`curl -s http://169.254.169.254/latest/meta-data/placement/availability-zone`
region=`echo $az|sed 's/[a-d]$//'`
suffix=`echo $region|tr '[:lower:]' '[:upper:]'|sed -r 's/-/_/g'`
echo "OSC_SUBREGION_NAME=$az" | tee -a $GITHUB_ENV
echo "OSC_REGION=$region" | tee -a $GITHUB_ENV
echo "OSC_ACCESS_KEY_NAME=OSC_ACCESS_KEY_$suffix" | tee -a $GITHUB_ENV
echo "OSC_SECRET_KEY_NAME=OSC_SECRET_KEY_$suffix" | tee -a $GITHUB_ENV
echo "OMI_ID_NAME=OMI_ID_$suffix" | tee -a $GITHUB_ENV
echo "IMG_UPGRADE_FROM_NAME=IMG_UPGRADE_FROM_$suffix" | tee -a $GITHUB_ENV
echo "IMG_UPGRADE_TO_NAME=IMG_UPGRADE_TO_$suffix" | tee -a $GITHUB_ENV
echo "IMG_MANAGEMENT_NAME=IMG_MANAGEMENT_$suffix" | tee -a $GITHUB_ENV
