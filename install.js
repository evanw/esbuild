

function getBinary(osType='Darwin'){
    const fs = require ('fs')
    const path = require('path')
    console.log(osType)
    const filterString = osType.toLowerCase()
    const regex = new RegExp(filterString)
    const startDirectory = './npm/'
    const filesFound = fs.readdirSync(startDirectory)
    for (let i = 0; i<filesFound.length;i++){
      if (filesFound[i].match(regex)){
        return filesFound[i]
      }
    }
  return null
}

function detectOS(){
  const os = require('os')
  return os.type();  
}

function installBinaries(){
  const osType = detectOS();
  const binaryToInstall = getBinary(osType)
  const { exec } = require('child_process');
  if (!binaryToInstall){
    console.log('No binaries found for your OS')
    return
  }
 console.log('Found the following file to install:',binaryToInstall)
 console.log('Installing the binaries now:') 
 exec(`npm install -g ${binaryToInstall}`)
 console.log(`Installation Finished, 
   \nplease read documentation on how to use esbuild\n
   https://github.com/evanw/esbuild`)
}

console.log('Staring the install Scripts');
installBinaries()


